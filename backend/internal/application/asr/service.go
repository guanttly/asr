package asr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	domain "github.com/lgt/asr/internal/domain/asr"
)

// Service orchestrates transcription use cases.
type Service struct {
	taskRepo          domain.TaskRepository
	batchSubmitter    BatchEngine
	hotwordProvider   HotwordProvider
	streamingEngine   StreamingEngine
	streamSessions    sync.Map
	streamSessionTTL  time.Duration
	streamSessionIDFn func() string
	postProcessor     CompletedTaskProcessor
	retryHistoryLimit int
	inflightTasks     sync.Map
	eventPublisher    EventPublisher
}

// EventPublisher emits user-scoped business events to interested websocket clients.
type EventPublisher interface {
	PublishUserEvent(userID uint64, topic string, payload any)
}

// CompletedTaskProcessor materializes completed batch tasks into downstream resources.
type CompletedTaskProcessor interface {
	ProcessCompletedTask(ctx context.Context, task *domain.TranscriptionTask) error
	ResumeCompletedTaskFromFailure(ctx context.Context, task *domain.TranscriptionTask) error
}

// HotwordProvider derives upstream ASR hotwords for a terminology dictionary.
type HotwordProvider interface {
	HotwordsForDict(ctx context.Context, dictID uint64) ([]string, error)
}

// BatchEngine describes the upstream batch ASR submission contract.
type BatchEngine interface {
	SubmitBatch(ctx context.Context, req BatchSubmitRequest) (*BatchSubmitResult, error)
	QueryBatchTask(ctx context.Context, taskID string) (*BatchTaskStatus, error)
}

// StreamingEngine describes the upstream streaming ASR contract.
type StreamingEngine interface {
	StartStreamSession(ctx context.Context) (string, error)
	PushStreamChunk(ctx context.Context, sessionID string, pcmData []byte) (*StreamChunkResponse, error)
	FinishStreamSession(ctx context.Context, sessionID string) (*StreamChunkResponse, error)
	// StreamingAvailable reports whether the upstream streaming endpoint is configured and usable.
	StreamingAvailable() bool
}

// BatchSubmitRequest is the application-level payload sent to the ASR engine.
type BatchSubmitRequest struct {
	AudioURL      string
	LocalFilePath string
	DictID        *uint64
	Language      string
	UseITN        *bool
	Hotwords      []string
	Progress      func(BatchSubmitProgress)
}

// BatchSubmitProgress describes segment-based progress for locally split uploads.
type BatchSubmitProgress struct {
	SegmentTotal     int
	SegmentCompleted int
}

// BatchSubmitResult is the engine response needed by the application layer.
type BatchSubmitResult struct {
	TaskID     string
	Status     string
	ResultText string
	Duration   float64
}

// BatchTaskStatus is the normalized state returned by the upstream ASR engine.
type BatchTaskStatus struct {
	Status     string
	ResultText string
	Duration   float64
}

// BatchSyncSummary describes one backend sync tick result.
type BatchSyncSummary struct {
	Scanned int
	Updated int
	Failed  int
	Alerts  []TaskSyncAlert
}

// TaskSyncAlert describes a task that has reached a repeated sync failure threshold.
type TaskSyncAlert struct {
	TaskID         uint64
	ExternalTaskID string
	FailCount      int
	LastSyncError  string
	NextSyncAt     *time.Time
}

const (
	baseSyncBackoff    = 30 * time.Second
	maxSyncBackoff     = 10 * time.Minute
	batchSubmitTimeout = 30 * time.Minute
	snippetPollTimeout = 2 * time.Minute
	snippetPollEvery   = 1200 * time.Millisecond
	DefaultLanguage    = "auto"
	// maxTaskSyncFailures 是批量任务同步/提交失败的最大重试次数，超过后任务收敛到
	// 终态 failed，避免错误音频 URL 等场景任务永远卡在 processing 既不失败也删不掉。
	maxTaskSyncFailures = 5
)

var transcriptionMarkupPattern = regexp.MustCompile(`(?i)language\s+[a-z_-]+<asr_text>|</?asr_text>`)
var transcriptionTokenPattern = regexp.MustCompile(`<\|[^>]+\|>`)
var transcriptionWhitespacePattern = regexp.MustCompile(`[\t\f\r ]+`)
var transcriptionClauseNumberPattern = regexp.MustCompile(`^[第]?[零〇一二三四五六七八九十百千万两\d]+[、.．，,：:]+`)

var ErrTaskNotFound = errors.New("task not found")
var ErrTaskDeleteNotAllowed = errors.New("only completed or failed tasks can be deleted")
var ErrTaskResumeNotAllowed = errors.New("only completed batch tasks with failed workflow post-process can continue from the failed node")

// NewService creates a new ASR application service.
func NewService(taskRepo domain.TaskRepository, batchSubmitter BatchEngine, postProcessor CompletedTaskProcessor, retryHistoryLimit int, eventPublisher EventPublisher) *Service {
	if retryHistoryLimit <= 0 {
		retryHistoryLimit = 5
	}
	service := &Service{
		taskRepo:          taskRepo,
		batchSubmitter:    batchSubmitter,
		postProcessor:     postProcessor,
		retryHistoryLimit: retryHistoryLimit,
		eventPublisher:    eventPublisher,
		streamSessionTTL:  defaultStreamSessionTTL,
		streamSessionIDFn: generateRandomStreamSessionID,
	}
	if streamingEngine, ok := batchSubmitter.(StreamingEngine); ok {
		service.streamingEngine = streamingEngine
	}
	return service
}

// SetHotwordProvider configures terminology-derived ASR hotwords.
func (s *Service) SetHotwordProvider(provider HotwordProvider) {
	s.hotwordProvider = provider
}

func (s *Service) newBatchSubmitRequest(ctx context.Context, localFilePath, audioURL string, dictID *uint64, language string, useITN *bool, hotwords []string, progress func(BatchSubmitProgress)) (BatchSubmitRequest, error) {
	normalizedLanguage, err := NormalizeLanguage(language)
	if err != nil {
		return BatchSubmitRequest{}, err
	}
	mergedHotwords := normalizeHotwords(hotwords)
	if s.hotwordProvider != nil && dictID != nil && *dictID > 0 {
		dictHotwords, err := s.hotwordProvider.HotwordsForDict(ctx, *dictID)
		if err != nil {
			return BatchSubmitRequest{}, err
		}
		mergedHotwords = mergeHotwords(mergedHotwords, dictHotwords)
	}

	return BatchSubmitRequest{
		AudioURL:      strings.TrimSpace(audioURL),
		LocalFilePath: strings.TrimSpace(localFilePath),
		DictID:        dictID,
		Language:      normalizedLanguage,
		UseITN:        useITN,
		Hotwords:      mergedHotwords,
		Progress:      progress,
	}, nil
}

func NormalizeLanguage(value string) (string, error) {
	language := strings.TrimSpace(value)
	if language == "" || strings.EqualFold(language, DefaultLanguage) {
		return DefaultLanguage, nil
	}
	switch strings.ToLower(strings.ReplaceAll(language, "_", "-")) {
	case "zh-cn", "zh-hans", "zh":
		return "zh", nil
	case "en-us", "en-gb", "en":
		return "en", nil
	}
	return language, nil
}

// IsSupportedLanguage reports whether value is a language code the ASR engine accepts.
// An empty value falls back to the default language and is considered valid.
func IsSupportedLanguage(value string) bool {
	language := strings.TrimSpace(value)
	if language == "" {
		return true
	}
	switch strings.ToLower(strings.ReplaceAll(language, "_", "-")) {
	case "auto", "zh-cn", "zh-hans", "zh", "en-us", "en-gb", "en":
		return true
	default:
		return false
	}
}

func normalizeHotwords(values []string) []string {
	return mergeHotwords(nil, values)
}

func mergeHotwords(left []string, right []string) []string {
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(left)+len(right))
	appendWord := func(value string) {
		word := strings.TrimSpace(value)
		if word == "" {
			return
		}
		if _, ok := seen[word]; ok {
			return
		}
		seen[word] = struct{}{}
		merged = append(merged, word)
	}
	for _, word := range left {
		appendWord(word)
	}
	for _, word := range right {
		appendWord(word)
	}
	return merged
}

// CreateTask creates a new batch transcription task.
func (s *Service) CreateTask(ctx context.Context, userID uint64, req *CreateTaskRequest) (*TaskResponse, error) {
	audioURL := strings.TrimSpace(req.AudioURL)
	resultText := strings.TrimSpace(req.ResultText)
	language, err := NormalizeLanguage(req.Language)
	if err != nil {
		return nil, err
	}

	if req.Type == domain.TaskTypeBatch && audioURL == "" {
		return nil, fmt.Errorf("audio_url is required for batch transcription")
	}
	if req.Type == domain.TaskTypeRealtime && strings.TrimSpace(req.StreamSessionID) != "" {
		localFilePath, duration, err := s.consumeManagedStreamAudio(strings.TrimSpace(req.StreamSessionID))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.LocalFilePath) == "" {
			req.LocalFilePath = localFilePath
		}
		if req.Duration <= 0 {
			req.Duration = duration
		}
	}
	if req.Type == domain.TaskTypeRealtime && resultText == "" {
		return nil, fmt.Errorf("result_text is required for realtime transcription")
	}

	task := &domain.TranscriptionTask{
		UserID:            userID,
		Type:              req.Type,
		Status:            domain.TaskStatusPending,
		PostProcessStatus: domain.PostProcessPending,
		AudioURL:          audioURL,
		LocalFilePath:     strings.TrimSpace(req.LocalFilePath),
		ResultText:        resultText,
		Duration:          req.Duration,
		DictID:            req.DictID,
		WorkflowID:        req.WorkflowID,
		Language:          language,
		UseITN:            req.UseITN,
		Hotwords:          normalizeHotwords(req.Hotwords),
	}

	if req.Type == domain.TaskTypeRealtime {
		task.Status = domain.TaskStatusCompleted
		task.PostProcessStatus = domain.PostProcessPending
	}

	if err := s.taskRepo.Create(ctx, task); err != nil {
		return nil, err
	}
	s.publishTaskUpdated(task)

	if req.Type == domain.TaskTypeRealtime {
		now := task.CreatedAt
		task.PostProcessedAt = &now

		if s.postProcessor != nil {
			task.PostProcessStatus = domain.PostProcessProcessing
			if err := s.postProcessor.ProcessCompletedTask(ctx, task); err != nil {
				task.PostProcessStatus = domain.PostProcessFailed
				task.PostProcessError = err.Error()
			} else {
				task.PostProcessStatus = domain.PostProcessCompleted
			}
		} else {
			task.PostProcessStatus = domain.PostProcessCompleted
		}

		if err := s.taskRepo.Update(ctx, task); err != nil {
			return nil, err
		}
		s.publishTaskUpdated(task)
		return ToResponse(task), nil
	}

	if req.Type == domain.TaskTypeBatch {
		if s.batchSubmitter == nil {
			task.ResultText = "asr batch engine is not configured"
			task.TransitionTo(domain.TaskStatusFailed)
			_ = s.taskRepo.Update(ctx, task)
			s.publishTaskUpdated(task)
			return nil, fmt.Errorf("asr batch engine is not configured")
		}

		s.dispatchBatchTask(task.ID)
	}

	return ToResponse(task), nil
}

// TranscribeSnippet submits a short local audio file and waits for the final text.
func (s *Service) TranscribeSnippet(ctx context.Context, req *TranscribeSnippetRequest) (*TranscribeSnippetResponse, error) {
	if req == nil || strings.TrimSpace(req.LocalFilePath) == "" {
		return nil, fmt.Errorf("local audio file is required")
	}
	if s.batchSubmitter == nil {
		return nil, fmt.Errorf("asr batch engine is not configured")
	}

	submitReq, err := s.newBatchSubmitRequest(ctx, strings.TrimSpace(req.LocalFilePath), "", req.DictID, req.Language, req.UseITN, req.Hotwords, nil)
	if err != nil {
		return nil, err
	}
	result, err := s.batchSubmitter.SubmitBatch(ctx, submitReq)
	if err != nil {
		return nil, err
	}

	return s.awaitSnippetResult(ctx, result)
}

// streamingAvailable reports whether realtime streaming ASR is configured and usable.
func (s *Service) streamingAvailable() bool {
	return s.streamingEngine != nil && s.streamingEngine.StreamingAvailable()
}

// StartStreamSession creates a backend-managed streaming facade session.
func (s *Service) StartStreamSession(ctx context.Context) (*StreamSessionResponse, error) {
	if !s.streamingAvailable() && s.batchSubmitter == nil {
		return nil, ErrStreamEngineUnavailable
	}

	now := time.Now()
	s.cleanupExpiredStreamSessions(now)
	upstreamSessionID := ""
	if s.streamingAvailable() {
		sessionID, err := s.streamingEngine.StartStreamSession(ctx)
		if err != nil {
			return nil, err
		}
		upstreamSessionID = sessionID
	}

	managedSessionID := s.newStreamSessionID()
	s.streamSessions.Store(managedSessionID, s.newManagedStreamSession(upstreamSessionID, now))

	return &StreamSessionResponse{SessionID: managedSessionID}, nil
}

// PushStreamChunk accepts one PCM chunk into the active session.
func (s *Service) PushStreamChunk(ctx context.Context, req *PushStreamChunkRequest) (*StreamChunkResponse, error) {
	if !s.streamingAvailable() && s.batchSubmitter == nil {
		return nil, ErrStreamEngineUnavailable
	}
	if req == nil || strings.TrimSpace(req.SessionID) == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if len(req.PCMData) == 0 {
		return nil, fmt.Errorf("audio chunk is required")
	}

	now := time.Now()
	sessionID := strings.TrimSpace(req.SessionID)
	session, err := s.loadManagedStreamSession(sessionID, now)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	if session.finalized {
		session.mu.Unlock()
		return nil, ErrStreamSessionClosed
	}

	session.pcmData = append(session.pcmData, req.PCMData...)
	session.durationSeconds = pcmBytesToDurationSeconds(session.pcmData)
	upstreamSessionID := strings.TrimSpace(session.upstreamSessionID)
	language := session.language
	useStreaming := s.streamingAvailable() && upstreamSessionID != ""
	if !useStreaming {
		session.expiresAt = now.Add(s.streamSessionTTL)
	}
	session.mu.Unlock()

	var result *StreamChunkResponse
	if useStreaming {
		result, err = s.streamingEngine.PushStreamChunk(ctx, upstreamSessionID, req.PCMData)
	} else {
		result, err = s.transcribeStreamPCMChunk(ctx, sessionID, req.PCMData, language)
	}
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.finalized {
		return nil, ErrStreamSessionClosed
	}
	return s.applyStreamChunkResult(sessionID, session, result, now, false), nil
}

// CommitStreamSegment commits the current cumulative streaming transcript since the last sentence boundary.
func (s *Service) CommitStreamSegment(ctx context.Context, sessionID string) (*StreamChunkResponse, error) {
	if !s.streamingAvailable() && s.batchSubmitter == nil {
		return nil, ErrStreamEngineUnavailable
	}
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	now := time.Now()
	session, err := s.loadManagedStreamSession(trimmedSessionID, now)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	return s.applyStreamCommitResult(trimmedSessionID, session, now, false), nil
}

// FinishStreamSession finalizes an active upstream streaming session.
func (s *Service) FinishStreamSession(ctx context.Context, sessionID string) (*StreamChunkResponse, error) {
	if !s.streamingAvailable() && s.batchSubmitter == nil {
		return nil, ErrStreamEngineUnavailable
	}
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	now := time.Now()
	session, err := s.loadManagedStreamSession(trimmedSessionID, now)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.finalized {
		return s.applyStreamCommitResult(trimmedSessionID, session, now, true), nil
	}

	upstreamSessionID := strings.TrimSpace(session.upstreamSessionID)
	if s.streamingAvailable() && upstreamSessionID != "" {
		result, err := s.streamingEngine.FinishStreamSession(ctx, upstreamSessionID)
		if err != nil {
			return nil, err
		}
		_ = s.applyStreamChunkResult(trimmedSessionID, session, result, now, true)
	}
	session.finalized = true
	if _, _, err := s.materializeManagedStreamAudio(trimmedSessionID, session); err != nil && !errors.Is(err, ErrStreamSessionEmptyAudio) {
		return nil, err
	}
	return s.applyStreamCommitResult(trimmedSessionID, session, now, true), nil
}

// GetTask retrieves a task by ID.
func (s *Service) GetTask(ctx context.Context, userID, id uint64) (*TaskResponse, error) {
	task, err := s.getOwnedTask(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	return ToResponse(task), nil
}

// GetTaskAdmin returns a task without user ownership filtering (admin scope).
func (s *Service) GetTaskAdmin(ctx context.Context, id uint64) (*TaskResponse, error) {
	task, err := s.resolveTask(ctx, 0, id, true)
	if err != nil {
		return nil, err
	}

	return ToResponse(task), nil
}

// DeleteTask removes a finished transcription task from the user's history.
func (s *Service) DeleteTask(ctx context.Context, userID, id uint64) error {
	return s.deleteTaskInternal(ctx, userID, id, false)
}

// DeleteTaskAdmin removes a finished transcription task regardless of owner (admin scope).
func (s *Service) DeleteTaskAdmin(ctx context.Context, id uint64) error {
	return s.deleteTaskInternal(ctx, 0, id, true)
}

func (s *Service) deleteTaskInternal(ctx context.Context, userID, id uint64, allowAny bool) error {
	task, err := s.resolveTask(ctx, userID, id, allowAny)
	if err != nil {
		return err
	}
	if !canDeleteTask(task) {
		return ErrTaskDeleteNotAllowed
	}
	if err := s.taskRepo.Delete(ctx, id); err != nil {
		return err
	}
	if localFilePath := strings.TrimSpace(task.LocalFilePath); localFilePath != "" {
		removeErr := os.Remove(localFilePath)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			// Ignore best-effort cleanup failures after the database record is gone.
		}
	}
	return nil
}

// ClearTasks removes deletable transcription tasks from the user's history.
func (s *Service) ClearTasks(ctx context.Context, userID uint64, taskType *domain.TaskType) (*ClearTasksResponse, error) {
	const batchSize = 100

	collected := make([]*domain.TranscriptionTask, 0, batchSize)
	skippedCount := 0
	offset := 0

	for {
		tasks, _, err := s.taskRepo.ListByUser(ctx, userID, taskType, offset, batchSize)
		if err != nil {
			return nil, err
		}
		if len(tasks) == 0 {
			break
		}

		for _, task := range tasks {
			if canDeleteTask(task) {
				collected = append(collected, task)
				continue
			}
			skippedCount++
		}

		if len(tasks) < batchSize {
			break
		}
		offset += len(tasks)
	}

	deletedCount := 0
	for _, task := range collected {
		if err := s.taskRepo.Delete(ctx, task.ID); err != nil {
			return nil, err
		}
		deletedCount++
		if localFilePath := strings.TrimSpace(task.LocalFilePath); localFilePath != "" {
			removeErr := os.Remove(localFilePath)
			if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				// Ignore best-effort cleanup failures after the database record is gone.
			}
		}
	}

	return &ClearTasksResponse{
		DeletedCount: deletedCount,
		SkippedCount: skippedCount,
	}, nil
}

// AdminSyncTask refreshes a batch task status without user ownership filtering.
func (s *Service) AdminSyncTask(ctx context.Context, id uint64) (*TaskResponse, error) {
	task, err := s.taskRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if shouldDispatchBatchTask(task) {
		s.dispatchBatchTask(task.ID)
		return ToResponse(task), nil
	}

	if _, err := s.syncTaskState(ctx, task); err != nil {
		return nil, err
	}

	return ToResponse(task), nil
}

// SyncTask refreshes a batch task status from the upstream ASR engine.
func (s *Service) SyncTask(ctx context.Context, userID, id uint64) (*TaskResponse, error) {
	task, err := s.getOwnedTask(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if shouldDispatchBatchTask(task) {
		s.dispatchBatchTask(task.ID)
		return ToResponse(task), nil
	}

	if _, err := s.syncTaskState(ctx, task); err != nil {
		return nil, err
	}

	return ToResponse(task), nil
}

// ResumeTaskPostProcessFromFailure continues a failed batch workflow from the failed node.
func (s *Service) ResumeTaskPostProcessFromFailure(ctx context.Context, userID, id uint64) (*TaskResponse, error) {
	return s.resumeTaskPostProcessInternal(ctx, userID, id, false)
}

// ResumeTaskPostProcessFromFailureAdmin resumes a failed batch workflow regardless of owner (admin scope).
func (s *Service) ResumeTaskPostProcessFromFailureAdmin(ctx context.Context, id uint64) (*TaskResponse, error) {
	return s.resumeTaskPostProcessInternal(ctx, 0, id, true)
}

func (s *Service) resumeTaskPostProcessInternal(ctx context.Context, userID, id uint64, allowAny bool) (*TaskResponse, error) {
	task, err := s.resolveTask(ctx, userID, id, allowAny)
	if err != nil {
		return nil, err
	}
	if task.Type != domain.TaskTypeBatch || task.Status != domain.TaskStatusCompleted || task.WorkflowID == nil || task.PostProcessStatus != domain.PostProcessFailed {
		return nil, ErrTaskResumeNotAllowed
	}
	if task.MeetingID != nil {
		return nil, ErrTaskResumeNotAllowed
	}
	if s.postProcessor == nil {
		return nil, fmt.Errorf("workflow post processor is not configured")
	}

	now := time.Now()
	task.PostProcessStatus = domain.PostProcessProcessing
	task.PostProcessError = ""

	resumeErr := s.postProcessor.ResumeCompletedTaskFromFailure(ctx, task)
	if resumeErr != nil {
		task.PostProcessStatus = domain.PostProcessFailed
		task.PostProcessError = resumeErr.Error()
		task.PostProcessedAt = nil
		s.recordSyncFailure(task, now, resumeErr)
		if updateErr := s.taskRepo.Update(ctx, task); updateErr != nil {
			return nil, updateErr
		}
		s.publishTaskUpdated(task)
		return ToResponse(task), nil
	}

	_ = s.recordSyncSuccess(task, now)
	s.markPostProcessCompleted(task, now)
	if err := s.taskRepo.Update(ctx, task); err != nil {
		return nil, err
	}
	s.publishTaskUpdated(task)

	return ToResponse(task), nil
}

// SyncPendingTasks refreshes a batch of local tasks from the upstream ASR engine.
func (s *Service) SyncPendingTasks(ctx context.Context, limit int) (*BatchSyncSummary, error) {
	tasks, err := s.taskRepo.ListSyncCandidates(ctx, limit)
	if err != nil {
		return nil, err
	}

	summary := &BatchSyncSummary{}
	for _, task := range tasks {
		summary.Scanned++
		updated, err := s.syncTaskState(ctx, task)
		if err != nil {
			summary.Failed++
			if shouldAlertOnSyncFailure(task.SyncFailCount) {
				summary.Alerts = append(summary.Alerts, TaskSyncAlert{
					TaskID:         task.ID,
					ExternalTaskID: task.ExternalTaskID,
					FailCount:      task.SyncFailCount,
					LastSyncError:  task.LastSyncError,
					NextSyncAt:     task.NextSyncAt,
				})
			}
			continue
		}
		if updated {
			summary.Updated++
		}
	}

	return summary, nil
}

// ListTasks returns a paginated list of tasks for a user.
func (s *Service) ListTasks(ctx context.Context, userID uint64, taskType *domain.TaskType, offset, limit int) (*TaskListResponse, error) {
	tasks, total, err := s.taskRepo.ListByUser(ctx, userID, taskType, offset, limit)
	if err != nil {
		return nil, err
	}

	items := make([]*TaskResponse, len(tasks))
	for i, task := range tasks {
		items[i] = ToResponse(task)
	}

	return &TaskListResponse{Items: items, Total: total}, nil
}

// ListAllTasks returns a paginated list of tasks across all users (admin scope).
func (s *Service) ListAllTasks(ctx context.Context, taskType *domain.TaskType, offset, limit int) (*TaskListResponse, error) {
	tasks, total, err := s.taskRepo.List(ctx, taskType, offset, limit)
	if err != nil {
		return nil, err
	}

	items := make([]*TaskResponse, len(tasks))
	for i, task := range tasks {
		items[i] = ToResponse(task)
	}

	return &TaskListResponse{Items: items, Total: total}, nil
}

// GetSyncHealth returns aggregated batch task sync health for the admin dashboard.
func (s *Service) GetSyncHealth(ctx context.Context, warnThreshold, alertLimit int) (*SyncHealthResponse, error) {
	overview, alerts, err := s.taskRepo.GetSyncHealth(ctx, warnThreshold, alertLimit)
	if err != nil {
		return nil, err
	}

	latestRetryResult, err := s.taskRepo.GetLatestRetryResult(ctx)
	if err != nil {
		return nil, err
	}
	retryHistory, err := s.taskRepo.GetRetryHistory(ctx, s.retryHistoryLimit)
	if err != nil {
		return nil, err
	}

	items := make([]SyncAlertResponse, len(alerts))
	for i, alert := range alerts {
		items[i] = SyncAlertResponse{
			TaskID:            alert.TaskID,
			ExternalTaskID:    alert.ExternalTaskID,
			MeetingID:         alert.MeetingID,
			AlertReason:       alert.AlertReason,
			Status:            alert.Status,
			PostProcessStatus: alert.PostProcessStatus,
			PostProcessError:  alert.PostProcessError,
			SyncFailCount:     alert.SyncFailCount,
			LastSyncError:     alert.LastSyncError,
			LastSyncAt:        alert.LastSyncAt,
			NextSyncAt:        alert.NextSyncAt,
			UpdatedAt:         alert.UpdatedAt,
		}
	}

	return &SyncHealthResponse{
		PendingCount:               overview.PendingCount,
		ProcessingCount:            overview.ProcessingCount,
		CompletedCount:             overview.CompletedCount,
		FailedCount:                overview.FailedCount,
		PostProcessPendingCount:    overview.PostProcessPendingCount,
		PostProcessProcessingCount: overview.PostProcessProcessingCount,
		PostProcessCompletedCount:  overview.PostProcessCompletedCount,
		PostProcessFailedCount:     overview.PostProcessFailedCount,
		RepeatedFailureCount:       overview.RepeatedFailureCount,
		LatestSyncAt:               overview.LatestSyncAt,
		LatestRetryResult:          toRetryPostProcessResponse(latestRetryResult),
		RetryHistory:               toRetryPostProcessResponses(retryHistory),
		Alerts:                     items,
	}, nil
}

// AdminRetryFailedPostProcess retries failed post-processing tasks in batch for the dashboard.
func (s *Service) AdminRetryFailedPostProcess(ctx context.Context, limit int, taskIDs []uint64) (*RetryPostProcessResponse, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 || normalizedLimit > 100 {
		normalizedLimit = 20
	}

	tasks, result, err := s.loadRetryTasks(ctx, normalizedLimit, taskIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	result.Limit = normalizedLimit
	result.RequestedTaskCount = len(taskIDs)
	result.CreatedAt = &now

	for _, task := range tasks {
		result.Scanned++
		updated, syncErr := s.syncTaskState(ctx, task)
		if syncErr != nil {
			result.Failed++
			result.Items = append(result.Items, RetryPostProcessItemResponse{
				TaskID:            task.ID,
				ExternalTaskID:    task.ExternalTaskID,
				MeetingID:         task.MeetingID,
				Outcome:           "failed",
				PostProcessStatus: task.PostProcessStatus,
				ErrorMessage:      syncErr.Error(),
			})
			continue
		}
		if updated {
			result.Updated++
		}

		outcome := "unchanged"
		if task.PostProcessStatus == domain.PostProcessCompleted {
			outcome = "completed"
		} else if updated {
			outcome = "updated"
		}

		result.Items = append(result.Items, RetryPostProcessItemResponse{
			TaskID:            task.ID,
			ExternalTaskID:    task.ExternalTaskID,
			MeetingID:         task.MeetingID,
			Outcome:           outcome,
			PostProcessStatus: task.PostProcessStatus,
			ErrorMessage:      task.PostProcessError,
		})
	}

	if err := s.taskRepo.SaveLatestRetryResult(ctx, toRetryPostProcessRecord(result), s.retryHistoryLimit); err != nil {
		return nil, err
	}

	return result, nil
}

// AdminClearRetryHistory removes persisted dashboard retry history.
func (s *Service) AdminClearRetryHistory(ctx context.Context) (*ClearRetryHistoryResponse, error) {
	if err := s.taskRepo.ClearRetryHistory(ctx); err != nil {
		return nil, err
	}
	return &ClearRetryHistoryResponse{Cleared: true}, nil
}

// AdminDeleteRetryHistoryItem removes one persisted dashboard retry-history record.
func (s *Service) AdminDeleteRetryHistoryItem(ctx context.Context, createdAt time.Time) (*DeleteRetryHistoryItemResponse, error) {
	if err := s.taskRepo.DeleteRetryHistoryItem(ctx, createdAt); err != nil {
		return nil, err
	}
	return &DeleteRetryHistoryItemResponse{Deleted: true}, nil
}

func (s *Service) loadRetryTasks(ctx context.Context, limit int, taskIDs []uint64) ([]*domain.TranscriptionTask, *RetryPostProcessResponse, error) {
	if len(taskIDs) == 0 {
		tasks, err := s.taskRepo.ListPostProcessRetryCandidates(ctx, limit)
		if err != nil {
			return nil, nil, err
		}
		return tasks, &RetryPostProcessResponse{Items: make([]RetryPostProcessItemResponse, 0, len(tasks))}, nil
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	maxItems := len(taskIDs)
	if maxItems > limit {
		maxItems = limit
	}
	selected := make([]*domain.TranscriptionTask, 0, maxItems)
	result := &RetryPostProcessResponse{Items: make([]RetryPostProcessItemResponse, 0, len(taskIDs))}
	seen := make(map[uint64]struct{}, len(taskIDs))

	for _, taskID := range taskIDs {
		if len(selected) >= limit {
			break
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}

		task, err := s.taskRepo.GetByID(ctx, taskID)
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, RetryPostProcessItemResponse{
				TaskID:       taskID,
				Outcome:      "skipped",
				ErrorMessage: err.Error(),
			})
			continue
		}

		if task.Type != domain.TaskTypeBatch || task.Status != domain.TaskStatusCompleted || task.PostProcessStatus != domain.PostProcessFailed {
			result.Items = append(result.Items, RetryPostProcessItemResponse{
				TaskID:            task.ID,
				ExternalTaskID:    task.ExternalTaskID,
				MeetingID:         task.MeetingID,
				Outcome:           "skipped",
				PostProcessStatus: task.PostProcessStatus,
				ErrorMessage:      "task is not eligible for post-process retry",
			})
			continue
		}

		selected = append(selected, task)
	}

	return selected, result, nil
}

func toRetryPostProcessResponse(record *domain.RetryPostProcessRecord) *RetryPostProcessResponse {
	if record == nil {
		return nil
	}

	createdAt := record.CreatedAt
	items := make([]RetryPostProcessItemResponse, len(record.Items))
	for i, item := range record.Items {
		items[i] = RetryPostProcessItemResponse{
			TaskID:            item.TaskID,
			ExternalTaskID:    item.ExternalTaskID,
			MeetingID:         item.MeetingID,
			Outcome:           item.Outcome,
			PostProcessStatus: item.PostProcessStatus,
			ErrorMessage:      item.ErrorMessage,
		}
	}

	return &RetryPostProcessResponse{
		Limit:              record.Limit,
		RequestedTaskCount: record.RequestedTaskCount,
		Scanned:            record.Scanned,
		Updated:            record.Updated,
		Failed:             record.Failed,
		CreatedAt:          &createdAt,
		Items:              items,
	}
}

func toRetryPostProcessResponses(records []*domain.RetryPostProcessRecord) []RetryPostProcessResponse {
	if len(records) == 0 {
		return nil
	}

	items := make([]RetryPostProcessResponse, 0, len(records))
	for _, record := range records {
		if dto := toRetryPostProcessResponse(record); dto != nil {
			items = append(items, *dto)
		}
	}
	return items
}

func toRetryPostProcessRecord(result *RetryPostProcessResponse) *domain.RetryPostProcessRecord {
	if result == nil {
		return nil
	}

	createdAt := time.Now()
	if result.CreatedAt != nil {
		createdAt = *result.CreatedAt
	}

	items := make([]domain.RetryPostProcessRecordItem, len(result.Items))
	for i, item := range result.Items {
		items[i] = domain.RetryPostProcessRecordItem{
			TaskID:            item.TaskID,
			ExternalTaskID:    item.ExternalTaskID,
			MeetingID:         item.MeetingID,
			Outcome:           item.Outcome,
			PostProcessStatus: item.PostProcessStatus,
			ErrorMessage:      item.ErrorMessage,
		}
	}

	return &domain.RetryPostProcessRecord{
		Limit:              result.Limit,
		RequestedTaskCount: result.RequestedTaskCount,
		Scanned:            result.Scanned,
		Updated:            result.Updated,
		Failed:             result.Failed,
		CreatedAt:          createdAt,
		Items:              items,
	}
}

func (s *Service) getOwnedTask(ctx context.Context, userID, id uint64) (*domain.TranscriptionTask, error) {
	return s.resolveTask(ctx, userID, id, false)
}

func (s *Service) resolveTask(ctx context.Context, userID, id uint64, allowAny bool) (*domain.TranscriptionTask, error) {
	task, err := s.taskRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, ErrTaskNotFound
	}
	if !allowAny && task.UserID != userID {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

func canDeleteTask(task *domain.TranscriptionTask) bool {
	if task == nil {
		return false
	}
	if task.Status == domain.TaskStatusFailed {
		return true
	}
	if task.Status != domain.TaskStatusCompleted {
		return false
	}
	if task.Type != domain.TaskTypeBatch {
		return true
	}
	return task.PostProcessStatus == domain.PostProcessCompleted || task.PostProcessStatus == domain.PostProcessFailed
}

func (s *Service) syncTaskState(ctx context.Context, task *domain.TranscriptionTask) (bool, error) {
	if task == nil {
		return false, nil
	}
	if !s.beginTaskRun(task.ID) {
		return false, nil
	}
	defer s.endTaskRun(task.ID)

	now := time.Now()

	if task.Type != domain.TaskTypeBatch {
		return false, nil
	}

	if task.Status == domain.TaskStatusFailed {
		return false, nil
	}

	if strings.TrimSpace(task.ExternalTaskID) == "" {
		if task.Status == domain.TaskStatusCompleted {
			return s.runCompletedTaskPostProcessing(ctx, task, now)
		}
		return s.submitBatchTask(ctx, task, now)
	}

	if task.Status == domain.TaskStatusCompleted {
		return s.runCompletedTaskPostProcessing(ctx, task, now)
	}

	if s.batchSubmitter == nil {
		return false, fmt.Errorf("asr batch engine is not configured")
	}

	status, err := s.batchSubmitter.QueryBatchTask(ctx, task.ExternalTaskID)
	if err != nil {
		s.handleSyncFailure(task, now, err)
		if updateErr := s.taskRepo.Update(ctx, task); updateErr != nil {
			return false, updateErr
		}
		s.publishTaskUpdated(task)
		return false, err
	}

	updated := false
	if s.recordSyncSuccess(task, now) {
		updated = true
	}
	if nextStatus := normalizeTaskStatus(status.Status); nextStatus != "" && nextStatus != task.Status {
		if task.TransitionTo(nextStatus) {
			updated = true
		}
	}

	if status.ResultText != "" && status.ResultText != task.ResultText {
		task.ResultText = sanitizeTranscriptionText(status.ResultText)
		updated = true
	}

	if status.Duration > 0 && status.Duration != task.Duration {
		task.Duration = status.Duration
		updated = true
	}

	if task.Status == domain.TaskStatusCompleted {
		processed, err := s.runCompletedTaskPostProcessing(ctx, task, now)
		if err != nil {
			return false, err
		}
		if processed {
			updated = true
		}
	}

	if updated {
		if err := s.taskRepo.Update(ctx, task); err != nil {
			return false, err
		}
		s.publishTaskUpdated(task)
	}

	return updated, nil
}

func (s *Service) submitBatchTask(ctx context.Context, task *domain.TranscriptionTask, now time.Time) (bool, error) {
	if s.batchSubmitter == nil {
		return false, fmt.Errorf("asr batch engine is not configured")
	}

	updated := false
	if task.Status != domain.TaskStatusProcessing {
		if task.TransitionTo(domain.TaskStatusProcessing) {
			updated = true
		}
	}
	if updated {
		if err := s.taskRepo.Update(ctx, task); err != nil {
			return false, err
		}
		s.publishTaskUpdated(task)
	}

	submitReq, err := s.newBatchSubmitRequest(ctx, task.LocalFilePath, task.AudioURL, task.DictID, task.Language, task.UseITN, task.Hotwords, func(progress BatchSubmitProgress) {
		s.updateTaskSegmentProgress(ctx, task, progress)
	})
	if err != nil {
		s.handleSyncFailure(task, now, err)
		if updateErr := s.taskRepo.Update(ctx, task); updateErr != nil {
			return false, updateErr
		}
		s.publishTaskUpdated(task)
		return false, err
	}

	result, err := s.batchSubmitter.SubmitBatch(ctx, submitReq)
	if err != nil {
		s.handleSyncFailure(task, now, err)
		if updateErr := s.taskRepo.Update(ctx, task); updateErr != nil {
			return false, updateErr
		}
		s.publishTaskUpdated(task)
		return false, err
	}

	if s.recordSyncSuccess(task, now) {
		updated = true
	}
	if strings.TrimSpace(result.TaskID) != "" {
		if task.SegmentTotal != 0 || task.SegmentCompleted != 0 {
			task.SegmentTotal = 0
			task.SegmentCompleted = 0
			updated = true
		}
	}

	if strings.TrimSpace(result.TaskID) != "" {
		if task.ExternalTaskID != result.TaskID {
			task.ExternalTaskID = result.TaskID
			updated = true
		}
		if task.Status != domain.TaskStatusProcessing {
			task.TransitionTo(domain.TaskStatusProcessing)
			updated = true
		}
	} else {
		status := normalizeTaskStatus(result.Status)
		if status == "" && strings.TrimSpace(result.ResultText) == "" && result.Duration <= 0 {
			err = fmt.Errorf("batch submission returned neither task_id nor result_text")
			s.handleSyncFailure(task, now, err)
			if updateErr := s.taskRepo.Update(ctx, task); updateErr != nil {
				return false, updateErr
			}
			s.publishTaskUpdated(task)
			return false, err
		}
		if status == "" {
			status = domain.TaskStatusCompleted
		}
		if status != task.Status {
			task.TransitionTo(status)
			updated = true
		}
		if result.ResultText != task.ResultText {
			task.ResultText = sanitizeTranscriptionText(result.ResultText)
			updated = true
		}
		if result.Duration > 0 && result.Duration != task.Duration {
			task.Duration = result.Duration
			updated = true
		}
	}
	if task.Status == domain.TaskStatusCompleted && task.SegmentTotal > 0 && task.SegmentCompleted != task.SegmentTotal {
		task.SegmentCompleted = task.SegmentTotal
		updated = true
	}

	if task.Status == domain.TaskStatusCompleted {
		processed, processErr := s.runCompletedTaskPostProcessing(ctx, task, now)
		if processErr != nil {
			return false, processErr
		}
		if processed {
			updated = true
		}
	}

	if updated {
		if err := s.taskRepo.Update(ctx, task); err != nil {
			return false, err
		}
		s.publishTaskUpdated(task)
	}

	return updated, nil
}

func (s *Service) updateTaskSegmentProgress(ctx context.Context, task *domain.TranscriptionTask, progress BatchSubmitProgress) {
	if task == nil || progress.SegmentTotal <= 0 {
		return
	}

	segmentTotal := progress.SegmentTotal
	segmentCompleted := progress.SegmentCompleted
	if segmentCompleted < 0 {
		segmentCompleted = 0
	}
	if segmentCompleted > segmentTotal {
		segmentCompleted = segmentTotal
	}

	changed := false
	if task.SegmentTotal != segmentTotal {
		task.SegmentTotal = segmentTotal
		changed = true
	}
	if task.SegmentCompleted != segmentCompleted {
		task.SegmentCompleted = segmentCompleted
		changed = true
	}
	if task.Status != domain.TaskStatusProcessing {
		if task.TransitionTo(domain.TaskStatusProcessing) {
			changed = true
		}
	}
	if !changed {
		return
	}

	if err := s.taskRepo.Update(ctx, task); err != nil {
		return
	}
	s.publishTaskUpdated(task)
}

func (s *Service) runCompletedTaskPostProcessing(ctx context.Context, task *domain.TranscriptionTask, now time.Time) (bool, error) {
	if task.MeetingID != nil {
		changed := s.markPostProcessCompleted(task, now)
		if s.recordSyncSuccess(task, now) {
			changed = true
		}
		if changed {
			if err := s.taskRepo.Update(ctx, task); err != nil {
				return false, err
			}
			s.publishTaskUpdated(task)
			return true, nil
		}
		return false, nil
	}

	if s.postProcessor == nil {
		return false, nil
	}

	// A completed task with an empty transcription result has nothing to
	// post-process (e.g. silent or speechless audio). Treat it as a terminal,
	// successful no-op instead of a retryable failure, otherwise the dashboard
	// "同步" action keeps looping forever on "empty transcription result".
	if strings.TrimSpace(task.ResultText) == "" {
		changed := s.recordSyncSuccess(task, now)
		if s.markPostProcessCompleted(task, now) {
			changed = true
		}
		if err := s.taskRepo.Update(ctx, task); err != nil {
			return false, err
		}
		s.publishTaskUpdated(task)
		return changed, nil
	}

	if task.PostProcessStatus != domain.PostProcessProcessing {
		task.PostProcessStatus = domain.PostProcessProcessing
		task.PostProcessError = ""
	}

	if err := s.postProcessor.ProcessCompletedTask(ctx, task); err != nil {
		task.PostProcessStatus = domain.PostProcessFailed
		task.PostProcessError = err.Error()
		task.PostProcessedAt = nil
		s.recordSyncFailure(task, now, err)
		if updateErr := s.taskRepo.Update(ctx, task); updateErr != nil {
			return false, updateErr
		}
		s.publishTaskUpdated(task)
		return false, err
	}

	changed := s.recordSyncSuccess(task, now)
	if s.markPostProcessCompleted(task, now) {
		changed = true
	}
	if err := s.taskRepo.Update(ctx, task); err != nil {
		return false, err
	}
	s.publishTaskUpdated(task)

	return changed, nil
}

func (s *Service) markPostProcessCompleted(task *domain.TranscriptionTask, now time.Time) bool {
	changed := false
	if task.PostProcessStatus != domain.PostProcessCompleted {
		task.PostProcessStatus = domain.PostProcessCompleted
		changed = true
	}
	if task.PostProcessError != "" {
		task.PostProcessError = ""
		changed = true
	}
	if task.PostProcessedAt == nil || !task.PostProcessedAt.Equal(now) {
		task.PostProcessedAt = &now
		changed = true
	}
	return changed
}

func (s *Service) recordSyncFailure(task *domain.TranscriptionTask, now time.Time, err error) {
	task.SyncFailCount++
	task.LastSyncAt = &now
	task.LastSyncError = err.Error()
	next := now.Add(syncBackoffDuration(task.SyncFailCount))
	task.NextSyncAt = &next
	task.UpdatedAt = now
}

// handleSyncFailure 记录一次批量同步失败，并在错误不可重试或重试次数超过上限时
// 将任务收敛到终态 failed，以便停止无限重试并允许删除。
func (s *Service) handleSyncFailure(task *domain.TranscriptionTask, now time.Time, err error) {
	s.recordSyncFailure(task, now, err)
	if !isNonRetryableTaskError(err) && task.SyncFailCount < maxTaskSyncFailures {
		return
	}
	if task.TransitionTo(domain.TaskStatusFailed) {
		task.NextSyncAt = nil
		task.UpdatedAt = now
	}
}

// isNonRetryableTaskError 判断批量同步失败是否属于重试也无法恢复的错误，
// 例如音频 URL 无效/返回 4xx、音频超过上游大小限制等客户端侧错误。
func isNonRetryableTaskError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "audio_url or local uploaded file is required"):
		return true
	case strings.Contains(msg, "音频文件超过 ASR 上游限制"):
		return true
	case strings.Contains(msg, "returned status 4"):
		// 音频源或上游返回 4xx 客户端错误（URL 失效、鉴权失败、参数错误等）。
		return true
	}
	return false
}

func (s *Service) recordSyncSuccess(task *domain.TranscriptionTask, now time.Time) bool {
	changed := false
	task.LastSyncAt = &now
	if task.NextSyncAt != nil {
		task.NextSyncAt = nil
		changed = true
	}
	if task.SyncFailCount != 0 {
		task.SyncFailCount = 0
		changed = true
	}
	if task.LastSyncError != "" {
		task.LastSyncError = ""
		changed = true
	}
	task.UpdatedAt = now
	return changed || task.LastSyncAt != nil
}

func syncBackoffDuration(failCount int) time.Duration {
	if failCount <= 0 {
		return baseSyncBackoff
	}

	multiplier := math.Pow(2, float64(failCount-1))
	backoff := time.Duration(float64(baseSyncBackoff) * multiplier)
	if backoff > maxSyncBackoff {
		return maxSyncBackoff
	}
	return backoff
}

func shouldAlertOnSyncFailure(failCount int) bool {
	if failCount < 3 {
		return false
	}

	return failCount == 3 || failCount%5 == 0
}

func shouldDispatchBatchTask(task *domain.TranscriptionTask) bool {
	if task == nil || task.Type != domain.TaskTypeBatch {
		return false
	}
	if task.Status == domain.TaskStatusFailed || task.Status == domain.TaskStatusCompleted {
		return false
	}
	return strings.TrimSpace(task.ExternalTaskID) == ""
}

func (s *Service) dispatchBatchTask(taskID uint64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), batchSubmitTimeout)
		defer cancel()

		task, err := s.taskRepo.GetByID(ctx, taskID)
		if err != nil {
			return
		}

		_, _ = s.syncTaskState(ctx, task)
	}()
}

func (s *Service) awaitSnippetResult(ctx context.Context, result *BatchSubmitResult) (*TranscribeSnippetResponse, error) {
	if result == nil {
		return nil, fmt.Errorf("empty asr response")
	}

	status := normalizeTaskStatus(result.Status)
	text := sanitizeTranscriptionText(result.ResultText)
	if status == domain.TaskStatusFailed {
		return nil, fmt.Errorf("snippet transcription failed")
	}
	if text != "" || strings.TrimSpace(result.TaskID) == "" {
		if status == "" {
			status = domain.TaskStatusCompleted
		}
		return &TranscribeSnippetResponse{
			Status:   string(status),
			Text:     text,
			Duration: result.Duration,
		}, nil
	}

	pollCtx, cancel := context.WithTimeout(ctx, snippetPollTimeout)
	defer cancel()

	ticker := time.NewTicker(snippetPollEvery)
	defer ticker.Stop()

	taskID := strings.TrimSpace(result.TaskID)
	for {
		statusResult, err := s.batchSubmitter.QueryBatchTask(pollCtx, taskID)
		if err != nil {
			return nil, err
		}

		status = normalizeTaskStatus(statusResult.Status)
		text = sanitizeTranscriptionText(statusResult.ResultText)
		if status == domain.TaskStatusFailed {
			return nil, fmt.Errorf("snippet transcription failed")
		}
		if text != "" || status == domain.TaskStatusCompleted {
			if status == "" {
				status = domain.TaskStatusCompleted
			}
			return &TranscribeSnippetResponse{
				Status:   string(status),
				Text:     text,
				Duration: statusResult.Duration,
			}, nil
		}

		select {
		case <-pollCtx.Done():
			return nil, fmt.Errorf("snippet transcription timed out")
		case <-ticker.C:
		}
	}
}

func (s *Service) beginTaskRun(taskID uint64) bool {
	if taskID == 0 {
		return false
	}
	_, loaded := s.inflightTasks.LoadOrStore(taskID, struct{}{})
	return !loaded
}

func (s *Service) endTaskRun(taskID uint64) {
	if taskID == 0 {
		return
	}
	s.inflightTasks.Delete(taskID)
}

func normalizeTaskStatus(status string) domain.TaskStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued", "created", "accepted":
		return domain.TaskStatusPending
	case "processing", "running", "in_progress", "in-progress", "started":
		return domain.TaskStatusProcessing
	case "completed", "done", "finished", "success", "succeeded":
		return domain.TaskStatusCompleted
	case "failed", "error", "cancelled", "canceled", "timeout":
		return domain.TaskStatusFailed
	default:
		return ""
	}
}

func sanitizeTranscriptionText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	cleaned := transcriptionMarkupPattern.ReplaceAllString(trimmed, "")
	cleaned = transcriptionTokenPattern.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "\u00a0", " ")
	lines := strings.Split(cleaned, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(transcriptionWhitespacePattern.ReplaceAllString(line, " "))
	}
	cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
	cleaned = collapseRepeatedRuns(cleaned)
	cleaned = collapseRunawayClauses(cleaned)
	return strings.TrimSpace(cleaned)
}

const (
	// 以下阈值仅用于折叠 ASR "复读机"式的连续重复幻觉，阈值偏高以避免
	// 误伤"好好好""哈哈哈"等正常表达；只有连续重复明显超出正常语言习惯才折叠。
	maxRepeatUnitRunes      = 40 // 检测的最大重复单元长度（按 rune 计）
	singleRuneRepeatTrigger = 16 // 单字连续重复达到该次数才折叠
	phraseRepeatTrigger     = 6  // 多字短语连续重复达到该次数才折叠
	repeatKeepCount         = 2  // 折叠后保留的重复次数
)

// collapseRepeatedRuns 折叠连续重复的字/词/短语（ASR 幻觉常见形态），
// 仅在重复次数明显超出正常语言习惯时才折叠，尽量避免误伤正常文本。
func collapseRepeatedRuns(text string) string {
	if text == "" {
		return text
	}
	runes := []rune(text)
	n := len(runes)
	out := make([]rune, 0, n)
	i := 0
	for i < n {
		matched := false
		maxUnit := maxRepeatUnitRunes
		if maxUnit > (n-i)/2 {
			maxUnit = (n - i) / 2
		}
		for unitLen := 1; unitLen <= maxUnit; unitLen++ {
			repeat := countRepeatedUnits(runes, i, unitLen)
			trigger := phraseRepeatTrigger
			if unitLen == 1 {
				trigger = singleRuneRepeatTrigger
			}
			if repeat < trigger {
				continue
			}
			keep := repeatKeepCount
			if keep > repeat {
				keep = repeat
			}
			for k := 0; k < keep; k++ {
				out = append(out, runes[i:i+unitLen]...)
			}
			i += repeat * unitLen
			matched = true
			break
		}
		if !matched {
			out = append(out, runes[i])
			i++
		}
	}
	return string(out)
}

// countRepeatedUnits 返回从 start 开始、长度为 unitLen 的单元连续重复的次数（含首次）。
func countRepeatedUnits(runes []rune, start, unitLen int) int {
	if unitLen <= 0 || start+unitLen > len(runes) {
		return 0
	}
	repeat := 1
	for j := start + unitLen; j+unitLen <= len(runes); j += unitLen {
		equal := true
		for k := 0; k < unitLen; k++ {
			if runes[j+k] != runes[start+k] {
				equal = false
				break
			}
		}
		if !equal {
			break
		}
		repeat++
	}
	return repeat
}

const (
	// 子句级防复读折叠的阈值：用于处理「整段多句循环重复」这类
	// collapseRepeatedRuns 无法覆盖的块级幻觉（重复单元跨句、超过 rune 上限）。
	// 阈值偏高，确保只有明显超出正常报告语言习惯的循环重复才会被折叠，
	// 对正常影像报告与会议纪要均安全（普通文本不会把一句话重复 4~6 次）。
	runawayClauseMinSegments = 8  // 子句总数不足时直接跳过，避免误伤短文本
	runawayClauseLongRunes   = 12 // 长句重复阈值：rune 数 ≥ 该值且重复 ≥ 4 次即折叠
	runawayClauseLongCount   = 4
	runawayClauseShortRunes  = 8 // 短句重复阈值：rune 数 ≥ 该值且重复 ≥ 6 次即折叠
	runawayClauseShortCount  = 6
)

// collapseRunawayClauses 折叠「整句/整段循环重复」式幻觉，仅保留首次出现。
// 与 collapseRepeatedRuns（处理连续重复的字/短语）互补：本函数按句切分后统计
// 归一化子句的出现频次，对明显超频的子句只保留第一次。此清洗始终生效，不依赖
// 工作流或词典配置，因此 OpenAPI 批量识别等未绑定工作流的链路也能抑制块级幻觉。
func collapseRunawayClauses(text string) string {
	if text == "" {
		return text
	}
	segments := splitTranscriptClauses(text)
	if len(segments) < runawayClauseMinSegments {
		return text
	}

	counts := make(map[string]int, len(segments))
	for _, segment := range segments {
		if key := normalizeClauseKey(segment); key != "" {
			counts[key]++
		}
	}

	var builder strings.Builder
	seen := make(map[string]int, len(counts))
	dropped := 0
	for _, segment := range segments {
		key := normalizeClauseKey(segment)
		if shouldDropRunawayClause(key, counts[key], seen[key]) {
			seen[key]++
			dropped++
			continue
		}
		if key != "" {
			seen[key]++
		}
		builder.WriteString(segment)
	}
	if dropped == 0 {
		return text
	}
	return strings.TrimSpace(builder.String())
}

// splitTranscriptClauses 按句末标点（。；;\n）切分文本，分隔符保留在前一子句末尾。
func splitTranscriptClauses(text string) []string {
	segments := make([]string, 0, 16)
	start := 0
	for index, value := range text {
		switch value {
		case '。', '；', ';', '\n':
			end := index + len(string(value))
			segments = append(segments, text[start:end])
			start = end
		}
	}
	if start < len(text) {
		segments = append(segments, text[start:])
	}
	return segments
}

// normalizeClauseKey 归一化子句用于频次统计：去除首尾标点、行内编号（如「1、」「2.」）
// 以及标点/空白，得到稳定的比较键；过短（<8 rune）的子句返回空串不参与统计。
func normalizeClauseKey(segment string) string {
	normalized := strings.TrimSpace(segment)
	normalized = strings.Trim(normalized, " \t\r\n，,。；;：:、.．")
	for {
		next := transcriptionClauseNumberPattern.ReplaceAllString(normalized, "")
		if next == normalized {
			break
		}
		normalized = strings.TrimSpace(next)
	}
	var builder strings.Builder
	for _, value := range normalized {
		switch value {
		case ' ', '\t', '\r', '\n', '，', ',', '。', '；', ';', '：', ':', '、', '.', '．':
			continue
		}
		builder.WriteRune(value)
	}
	normalized = builder.String()
	if len([]rune(normalized)) < runawayClauseShortRunes {
		return ""
	}
	return normalized
}

// shouldDropRunawayClause 判断某个归一化子句是否属于需折叠的循环重复幻觉。
// seenCount==0 表示这是首次出现，永远保留。
func shouldDropRunawayClause(key string, totalCount, seenCount int) bool {
	if key == "" || seenCount == 0 {
		return false
	}
	length := len([]rune(key))
	if length >= runawayClauseLongRunes && totalCount >= runawayClauseLongCount {
		return true
	}
	return length >= runawayClauseShortRunes && totalCount >= runawayClauseShortCount
}

func (s *Service) publishTaskUpdated(task *domain.TranscriptionTask) {
	if s == nil || s.eventPublisher == nil || task == nil || task.UserID == 0 {
		return
	}
	s.eventPublisher.PublishUserEvent(task.UserID, "asr.task.updated", map[string]any{
		"task": ToResponse(task),
	})
}
