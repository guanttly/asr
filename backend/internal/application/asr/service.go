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

// BatchEngine describes the upstream batch ASR submission contract.
type BatchEngine interface {
	SubmitBatch(ctx context.Context, req BatchSubmitRequest) (*BatchSubmitResult, error)
	QueryBatchTask(ctx context.Context, taskID string) (*BatchTaskStatus, error)
}

// BatchSubmitRequest is the application-level payload sent to the ASR engine.
type BatchSubmitRequest struct {
	AudioURL      string
	LocalFilePath string
	DictID        *uint64
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
)

var transcriptionMarkupPattern = regexp.MustCompile(`(?i)language\s+[a-z_-]+<asr_text>|</?asr_text>`)
var transcriptionTokenPattern = regexp.MustCompile(`<\|[^>]+\|>`)
var transcriptionWhitespacePattern = regexp.MustCompile(`[\t\f\r ]+`)

var ErrTaskNotFound = errors.New("task not found")
var ErrTaskDeleteNotAllowed = errors.New("only completed or failed tasks can be deleted")
var ErrTaskResumeNotAllowed = errors.New("only completed batch tasks with failed workflow post-process can continue from the failed node")

// NewService creates a new ASR application service.
func NewService(taskRepo domain.TaskRepository, batchSubmitter BatchEngine, postProcessor CompletedTaskProcessor, retryHistoryLimit int, eventPublisher EventPublisher) *Service {
	if retryHistoryLimit <= 0 {
		retryHistoryLimit = 5
	}
	return &Service{taskRepo: taskRepo, batchSubmitter: batchSubmitter, postProcessor: postProcessor, retryHistoryLimit: retryHistoryLimit, eventPublisher: eventPublisher}
}

// CreateTask creates a new batch transcription task.
func (s *Service) CreateTask(ctx context.Context, userID uint64, req *CreateTaskRequest) (*TaskResponse, error) {
	audioURL := strings.TrimSpace(req.AudioURL)
	resultText := strings.TrimSpace(req.ResultText)

	if req.Type == domain.TaskTypeBatch && audioURL == "" {
		return nil, fmt.Errorf("audio_url is required for batch transcription")
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

	result, err := s.batchSubmitter.SubmitBatch(ctx, BatchSubmitRequest{
		LocalFilePath: strings.TrimSpace(req.LocalFilePath),
		DictID:        req.DictID,
	})
	if err != nil {
		return nil, err
	}

	return s.awaitSnippetResult(ctx, result)
}

// GetTask retrieves a task by ID.
func (s *Service) GetTask(ctx context.Context, userID, id uint64) (*TaskResponse, error) {
	task, err := s.getOwnedTask(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	return ToResponse(task), nil
}

// DeleteTask removes a finished transcription task from the user's history.
func (s *Service) DeleteTask(ctx context.Context, userID, id uint64) error {
	task, err := s.getOwnedTask(ctx, userID, id)
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
	task, err := s.getOwnedTask(ctx, userID, id)
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
func (s *Service) ListTasks(ctx context.Context, userID uint64, offset, limit int) (*TaskListResponse, error) {
	tasks, total, err := s.taskRepo.ListByUser(ctx, userID, offset, limit)
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
	task, err := s.taskRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrTaskNotFound
	}
	if task.UserID != userID {
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
		s.recordSyncFailure(task, now, err)
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

	result, err := s.batchSubmitter.SubmitBatch(ctx, BatchSubmitRequest{
		AudioURL:      task.AudioURL,
		LocalFilePath: task.LocalFilePath,
		DictID:        task.DictID,
		Progress: func(progress BatchSubmitProgress) {
			s.updateTaskSegmentProgress(ctx, task, progress)
		},
	})
	if err != nil {
		s.recordSyncFailure(task, now, err)
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
			s.recordSyncFailure(task, now, err)
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
	return strings.TrimSpace(cleaned)
}

func (s *Service) publishTaskUpdated(task *domain.TranscriptionTask) {
	if s == nil || s.eventPublisher == nil || task == nil || task.UserID == 0 {
		return
	}
	s.eventPublisher.PublishUserEvent(task.UserID, "asr.task.updated", map[string]any{
		"task": ToResponse(task),
	})
}
