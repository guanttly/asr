package meeting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appwf "github.com/lgt/asr/internal/application/workflow"
	domain "github.com/lgt/asr/internal/domain/meeting"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"gorm.io/gorm"
)

var ErrMeetingNotFound = errors.New("meeting not found")
var ErrMeetingDeleteNotAllowed = errors.New("processing meeting cannot be deleted")

const (
	baseSyncBackoff    = 30 * time.Second
	maxSyncBackoff     = 10 * time.Minute
	batchSubmitTimeout = 30 * time.Minute
)

// SummaryWorkflowExecutor runs a workflow against meeting transcript text.
type SummaryWorkflowExecutor interface {
	ExecuteMeetingSummaryWorkflow(ctx context.Context, workflowID, meetingID, userID uint64, inputText, audioURL, audioFilePath string) (*appwf.ExecutionResponse, error)
}

// BatchEngine submits and queries long-running ASR jobs for meetings.
type BatchEngine interface {
	SubmitBatch(ctx context.Context, req BatchSubmitRequest) (*BatchSubmitResult, error)
	QueryBatchTask(ctx context.Context, taskID string) (*BatchTaskStatus, error)
}

// EventPublisher emits user-scoped business events.
type EventPublisher interface {
	PublishUserEvent(userID uint64, topic string, payload any)
}

// BatchSubmitRequest is the meeting-side request sent to the ASR engine.
type BatchSubmitRequest struct {
	AudioURL      string
	LocalFilePath string
}

// BatchSubmitResult is the minimal engine response needed by the meeting pipeline.
type BatchSubmitResult struct {
	TaskID     string
	Status     string
	ResultText string
	Duration   float64
}

// BatchTaskStatus is the polled state of an upstream ASR task.
type BatchTaskStatus struct {
	Status     string
	ResultText string
	Duration   float64
}

// SyncSummary describes one meeting sync tick.
type SyncSummary struct {
	Scanned int
	Updated int
	Failed  int
}

// Service orchestrates meeting use cases.
type Service struct {
	meetingRepo      domain.MeetingRepository
	transcriptRepo   domain.TranscriptRepository
	summaryRepo      domain.SummaryRepository
	workflowExec     SummaryWorkflowExecutor
	batchSubmitter   BatchEngine
	eventPublisher   EventPublisher
	inflightMeetings sync.Map
}

// NewService creates a new meeting application service.
func NewService(
	meetingRepo domain.MeetingRepository,
	transcriptRepo domain.TranscriptRepository,
	summaryRepo domain.SummaryRepository,
	workflowExec SummaryWorkflowExecutor,
	batchSubmitter BatchEngine,
	eventPublisher EventPublisher,
) *Service {
	return &Service{
		meetingRepo:    meetingRepo,
		transcriptRepo: transcriptRepo,
		summaryRepo:    summaryRepo,
		workflowExec:   workflowExec,
		batchSubmitter: batchSubmitter,
		eventPublisher: eventPublisher,
	}
}

// CreateMeeting creates a new meeting record.
func (s *Service) CreateMeeting(ctx context.Context, userID uint64, req *CreateMeetingRequest) (*MeetingResponse, error) {
	title := resolveMeetingTitle(req, time.Now())
	audioURL := strings.TrimSpace(req.AudioURL)
	if audioURL == "" {
		return nil, fmt.Errorf("audio_url is required")
	}

	m := &domain.Meeting{
		UserID:        userID,
		Title:         title,
		AudioURL:      audioURL,
		LocalFilePath: strings.TrimSpace(req.LocalFilePath),
		Duration:      req.Duration,
		WorkflowID:    req.WorkflowID,
		Status:        domain.MeetingStatusUploaded,
	}
	if err := s.meetingRepo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.publishMeetingUpdated(m)
	if s.batchSubmitter != nil {
		s.dispatchMeetingTask(m.ID)
	}
	return toMeetingResponse(m), nil
}

func defaultMeetingTitle(now time.Time) string {
	return now.Format("2006-01-02")
}

func resolveMeetingTitle(req *CreateMeetingRequest, now time.Time) string {
	if req == nil {
		return defaultMeetingTitle(now)
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		return title
	}
	if title := titleFromPath(req.LocalFilePath); title != "" {
		return title
	}
	if title := titleFromAudioURL(req.AudioURL); title != "" {
		return title
	}
	return defaultMeetingTitle(now)
}

func titleFromPath(filePath string) string {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return ""
	}
	base := filepath.Base(trimmed)
	name := strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
	return name
}

func titleFromAudioURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil {
		if name := strings.TrimSpace(strings.TrimSuffix(path.Base(parsed.Path), path.Ext(parsed.Path))); name != "" && name != "." && name != "/" {
			return name
		}
	}
	return ""
}

// GetMeeting retrieves meeting detail with transcripts and summary.
func (s *Service) GetMeeting(ctx context.Context, id uint64) (*MeetingDetailResponse, error) {
	m, err := s.meetingRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	transcripts, err := s.transcriptRepo.ListByMeeting(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]TranscriptItem, len(transcripts))
	for i, t := range transcripts {
		items[i] = TranscriptItem{
			SpeakerLabel: t.SpeakerLabel,
			Text:         t.Text,
			StartTime:    t.StartTime,
			EndTime:      t.EndTime,
		}
	}

	resp := &MeetingDetailResponse{
		MeetingResponse: *toMeetingResponse(m),
		Transcripts:     items,
	}

	summary, err := s.summaryRepo.GetByMeeting(ctx, id)
	if err == nil && summary != nil {
		resp.Summary = &SummaryItem{
			Content:      summary.Content,
			ModelVersion: summary.ModelVersion,
			CreatedAt:    summary.CreatedAt,
		}
	}

	return resp, nil
}

// ListMeetings returns a paginated list for a user.
func (s *Service) ListMeetings(ctx context.Context, userID uint64, offset, limit int) ([]*MeetingResponse, int64, error) {
	meetings, total, err := s.meetingRepo.List(ctx, userID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*MeetingResponse, len(meetings))
	for i, m := range meetings {
		items[i] = toMeetingResponse(m)
	}
	return items, total, nil
}

// SyncPendingMeetings refreshes uploaded or processing meetings from the upstream ASR engine.
func (s *Service) SyncPendingMeetings(ctx context.Context, limit int) (*SyncSummary, error) {
	meetings, err := s.meetingRepo.ListSyncCandidates(ctx, limit)
	if err != nil {
		return nil, err
	}

	summary := &SyncSummary{}
	for _, meeting := range meetings {
		summary.Scanned++
		updated, syncErr := s.syncMeetingState(ctx, meeting)
		if syncErr != nil {
			summary.Failed++
			continue
		}
		if updated {
			summary.Updated++
		}
	}

	return summary, nil
}

// DeleteMeeting removes a finished or stale meeting and its related transcript/summary data.
func (s *Service) DeleteMeeting(ctx context.Context, meetingID uint64, userID uint64) error {
	meeting, err := s.meetingRepo.GetByID(ctx, meetingID)
	if err != nil {
		return ErrMeetingNotFound
	}
	if meeting.UserID != userID {
		return ErrMeetingNotFound
	}
	if !canDeleteMeeting(meeting) {
		return ErrMeetingDeleteNotAllowed
	}
	if s.summaryRepo != nil {
		if err := s.summaryRepo.DeleteByMeeting(ctx, meetingID); err != nil {
			return err
		}
	}
	if s.transcriptRepo != nil {
		if err := s.transcriptRepo.DeleteByMeeting(ctx, meetingID); err != nil {
			return err
		}
	}
	if err := s.meetingRepo.Delete(ctx, meetingID); err != nil {
		return err
	}
	if localFilePath := strings.TrimSpace(meeting.LocalFilePath); localFilePath != "" {
		removeErr := os.Remove(localFilePath)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			// Ignore best-effort cleanup failures after the database record is gone.
		}
	}
	return nil
}

// RegenerateSummary executes the selected workflow against meeting transcript text and persists the meeting summary node output.
func (s *Service) RegenerateSummary(ctx context.Context, meetingID uint64, userID uint64, req *RegenerateSummaryRequest) (*MeetingDetailResponse, error) {
	meeting, err := s.meetingRepo.GetByID(ctx, meetingID)
	if err != nil {
		return nil, err
	}
	if meeting.UserID != userID {
		return nil, fmt.Errorf("meeting not found")
	}
	if s.workflowExec == nil {
		return nil, fmt.Errorf("workflow executor unavailable")
	}

	if req != nil && req.WorkflowID != nil {
		meeting.WorkflowID = req.WorkflowID
		if err := s.meetingRepo.Update(ctx, meeting); err != nil {
			return nil, err
		}
	}
	if meeting.WorkflowID == nil {
		return nil, fmt.Errorf("workflow_id is required")
	}

	transcripts, err := s.transcriptRepo.ListByMeeting(ctx, meetingID)
	if err != nil {
		return nil, err
	}
	if len(transcripts) == 0 {
		return nil, fmt.Errorf("meeting has no transcript")
	}

	inputText := buildSummaryInput(transcripts)
	exec, err := s.workflowExec.ExecuteMeetingSummaryWorkflow(ctx, *meeting.WorkflowID, meeting.ID, meeting.UserID, inputText, meeting.AudioURL, meeting.LocalFilePath)
	if err != nil {
		return nil, err
	}

	content, modelVersion, err := extractMeetingSummary(exec, *meeting.WorkflowID)
	if err != nil {
		return nil, err
	}

	existing, err := s.summaryRepo.GetByMeeting(ctx, meetingID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing != nil {
		existing.Content = content
		existing.ModelVersion = modelVersion
		if err := s.summaryRepo.Update(ctx, existing); err != nil {
			return nil, err
		}
	} else {
		if err := s.summaryRepo.Create(ctx, &domain.Summary{
			MeetingID:    meetingID,
			Content:      content,
			ModelVersion: modelVersion,
		}); err != nil {
			return nil, err
		}
	}
	s.publishMeetingUpdated(meeting)

	return s.GetMeeting(ctx, meetingID)
}

func (s *Service) syncMeetingState(ctx context.Context, meeting *domain.Meeting) (bool, error) {
	if meeting == nil {
		return false, nil
	}
	if meeting.Status == domain.MeetingStatusCompleted || meeting.Status == domain.MeetingStatusFailed {
		return false, nil
	}
	if !s.beginMeetingRun(meeting.ID) {
		return false, nil
	}
	defer s.endMeetingRun(meeting.ID)

	now := time.Now()
	if strings.TrimSpace(meeting.ExternalTaskID) == "" {
		return s.submitMeetingTask(ctx, meeting, now)
	}
	if s.batchSubmitter == nil {
		return s.persistMeetingSyncFailure(ctx, meeting, now, fmt.Errorf("asr batch engine is not configured"))
	}

	status, err := s.batchSubmitter.QueryBatchTask(ctx, meeting.ExternalTaskID)
	if err != nil {
		return s.persistMeetingSyncFailure(ctx, meeting, now, err)
	}

	updated := s.recordMeetingSyncSuccess(meeting, now)
	if nextStatus := normalizeMeetingStatus(status.Status); nextStatus != "" && nextStatus != meeting.Status {
		meeting.Status = nextStatus
		updated = true
	}
	if status.Duration > 0 && status.Duration != meeting.Duration {
		meeting.Duration = status.Duration
		updated = true
	}
	if meeting.Status == domain.MeetingStatusFailed {
		if msg := strings.TrimSpace(status.ResultText); msg != "" && meeting.LastSyncError != msg {
			meeting.LastSyncError = msg
			updated = true
		}
	}
	if meeting.Status == domain.MeetingStatusCompleted {
		completed, completeErr := s.finalizeCompletedMeeting(ctx, meeting, status.ResultText)
		if completeErr != nil {
			return s.failMeeting(ctx, meeting, now, completeErr)
		}
		if completed {
			updated = true
		}
	}

	if updated {
		if err := s.meetingRepo.Update(ctx, meeting); err != nil {
			return false, err
		}
		s.publishMeetingUpdated(meeting)
	}

	return updated, nil
}

func (s *Service) submitMeetingTask(ctx context.Context, meeting *domain.Meeting, now time.Time) (bool, error) {
	if s.batchSubmitter == nil {
		return s.persistMeetingSyncFailure(ctx, meeting, now, fmt.Errorf("asr batch engine is not configured"))
	}

	updated := false
	if meeting.Status != domain.MeetingStatusProcessing {
		meeting.Status = domain.MeetingStatusProcessing
		updated = true
	}
	if updated {
		if err := s.meetingRepo.Update(ctx, meeting); err != nil {
			return false, err
		}
		s.publishMeetingUpdated(meeting)
	}

	result, err := s.batchSubmitter.SubmitBatch(ctx, BatchSubmitRequest{
		AudioURL:      meeting.AudioURL,
		LocalFilePath: meeting.LocalFilePath,
	})
	if err != nil {
		return s.persistMeetingSyncFailure(ctx, meeting, now, err)
	}

	updated = s.recordMeetingSyncSuccess(meeting, now)
	if strings.TrimSpace(result.TaskID) != "" {
		if meeting.ExternalTaskID != result.TaskID {
			meeting.ExternalTaskID = result.TaskID
			updated = true
		}
		if meeting.Status != domain.MeetingStatusProcessing {
			meeting.Status = domain.MeetingStatusProcessing
			updated = true
		}
		if updated {
			if err := s.meetingRepo.Update(ctx, meeting); err != nil {
				return false, err
			}
			s.publishMeetingUpdated(meeting)
		}
		return updated, nil
	}

	status := normalizeMeetingStatus(result.Status)
	if status == "" && strings.TrimSpace(result.ResultText) == "" && result.Duration <= 0 {
		return s.persistMeetingSyncFailure(ctx, meeting, now, fmt.Errorf("batch submission returned neither task_id nor result_text"))
	}
	if status == "" {
		status = domain.MeetingStatusCompleted
	}
	if status != meeting.Status {
		meeting.Status = status
		updated = true
	}
	if result.Duration > 0 && result.Duration != meeting.Duration {
		meeting.Duration = result.Duration
		updated = true
	}
	if meeting.Status == domain.MeetingStatusFailed {
		if msg := strings.TrimSpace(result.ResultText); msg != "" && meeting.LastSyncError != msg {
			meeting.LastSyncError = msg
			updated = true
		}
	}
	if meeting.Status == domain.MeetingStatusCompleted {
		completed, completeErr := s.finalizeCompletedMeeting(ctx, meeting, result.ResultText)
		if completeErr != nil {
			return s.failMeeting(ctx, meeting, now, completeErr)
		}
		if completed {
			updated = true
		}
	}

	if updated {
		if err := s.meetingRepo.Update(ctx, meeting); err != nil {
			return false, err
		}
		s.publishMeetingUpdated(meeting)
	}

	return updated, nil
}

func (s *Service) finalizeCompletedMeeting(ctx context.Context, meeting *domain.Meeting, rawText string) (bool, error) {
	text := strings.TrimSpace(rawText)
	if text == "" && s.transcriptRepo != nil {
		transcripts, err := s.transcriptRepo.ListByMeeting(ctx, meeting.ID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
		if len(transcripts) > 0 {
			text = buildSummaryInput(transcripts)
		}
	}
	if text == "" {
		return false, fmt.Errorf("empty transcription result")
	}

	transcriptText := text
	summaryContent := ""
	modelVersion := ""
	if meeting.WorkflowID != nil {
		if s.workflowExec == nil {
			return false, fmt.Errorf("workflow executor unavailable")
		}
		exec, err := s.workflowExec.ExecuteMeetingSummaryWorkflow(ctx, *meeting.WorkflowID, meeting.ID, meeting.UserID, text, meeting.AudioURL, meeting.LocalFilePath)
		if err != nil {
			return false, err
		}
		transcriptText = extractMeetingTranscriptText(exec, text)
		content, version, err := extractMeetingSummary(exec, *meeting.WorkflowID)
		if err != nil {
			return false, err
		}
		summaryContent = content
		modelVersion = version
	}

	changed := false
	if s.transcriptRepo != nil {
		if err := s.transcriptRepo.DeleteByMeeting(ctx, meeting.ID); err != nil {
			return false, err
		}
		if err := s.transcriptRepo.BatchCreate(ctx, []domain.Transcript{{
			MeetingID:    meeting.ID,
			SpeakerLabel: "ASR",
			Text:         transcriptText,
			StartTime:    0,
			EndTime:      meeting.Duration,
		}}); err != nil {
			return false, err
		}
		changed = true
	}
	if summaryContent != "" && s.summaryRepo != nil {
		summaryChanged, err := s.upsertMeetingSummary(ctx, meeting.ID, summaryContent, modelVersion)
		if err != nil {
			return false, err
		}
		if summaryChanged {
			changed = true
		}
	}
	if meeting.Status != domain.MeetingStatusCompleted {
		meeting.Status = domain.MeetingStatusCompleted
		changed = true
	}
	return changed, nil
}

func (s *Service) upsertMeetingSummary(ctx context.Context, meetingID uint64, content, modelVersion string) (bool, error) {
	existing, err := s.summaryRepo.GetByMeeting(ctx, meetingID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if existing != nil {
		changed := false
		if existing.Content != content {
			existing.Content = content
			changed = true
		}
		if existing.ModelVersion != modelVersion {
			existing.ModelVersion = modelVersion
			changed = true
		}
		if !changed {
			return false, nil
		}
		if err := s.summaryRepo.Update(ctx, existing); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := s.summaryRepo.Create(ctx, &domain.Summary{
		MeetingID:    meetingID,
		Content:      content,
		ModelVersion: modelVersion,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) persistMeetingSyncFailure(ctx context.Context, meeting *domain.Meeting, now time.Time, err error) (bool, error) {
	s.recordMeetingSyncFailure(meeting, now, err)
	if updateErr := s.meetingRepo.Update(ctx, meeting); updateErr != nil {
		return false, updateErr
	}
	s.publishMeetingUpdated(meeting)
	return false, err
}

func (s *Service) failMeeting(ctx context.Context, meeting *domain.Meeting, now time.Time, err error) (bool, error) {
	updated := s.recordMeetingSyncSuccess(meeting, now)
	if meeting.Status != domain.MeetingStatusFailed {
		meeting.Status = domain.MeetingStatusFailed
		updated = true
	}
	message := strings.TrimSpace(err.Error())
	if message != "" && meeting.LastSyncError != message {
		meeting.LastSyncError = message
		updated = true
	}
	if updated {
		if updateErr := s.meetingRepo.Update(ctx, meeting); updateErr != nil {
			return false, updateErr
		}
		s.publishMeetingUpdated(meeting)
	}
	return false, err
}

func (s *Service) recordMeetingSyncFailure(meeting *domain.Meeting, now time.Time, err error) {
	meeting.SyncFailCount++
	meeting.LastSyncAt = &now
	meeting.LastSyncError = err.Error()
	next := now.Add(syncBackoffDuration(meeting.SyncFailCount))
	meeting.NextSyncAt = &next
	meeting.UpdatedAt = now
	if meeting.Status == domain.MeetingStatusUploaded {
		meeting.Status = domain.MeetingStatusProcessing
	}
}

func (s *Service) recordMeetingSyncSuccess(meeting *domain.Meeting, now time.Time) bool {
	changed := false
	meeting.LastSyncAt = &now
	if meeting.NextSyncAt != nil {
		meeting.NextSyncAt = nil
		changed = true
	}
	if meeting.SyncFailCount != 0 {
		meeting.SyncFailCount = 0
		changed = true
	}
	if meeting.LastSyncError != "" {
		meeting.LastSyncError = ""
		changed = true
	}
	meeting.UpdatedAt = now
	return changed || meeting.LastSyncAt != nil
}

func (s *Service) dispatchMeetingTask(meetingID uint64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), batchSubmitTimeout)
		defer cancel()

		meeting, err := s.meetingRepo.GetByID(ctx, meetingID)
		if err != nil {
			return
		}

		_, _ = s.syncMeetingState(ctx, meeting)
	}()
}

func (s *Service) beginMeetingRun(meetingID uint64) bool {
	if meetingID == 0 {
		return false
	}
	_, loaded := s.inflightMeetings.LoadOrStore(meetingID, struct{}{})
	return !loaded
}

func (s *Service) endMeetingRun(meetingID uint64) {
	if meetingID == 0 {
		return
	}
	s.inflightMeetings.Delete(meetingID)
}

func syncBackoffDuration(failCount int) time.Duration {
	if failCount <= 0 {
		return baseSyncBackoff
	}

	multiplier := 1 << (failCount - 1)
	backoff := time.Duration(multiplier) * baseSyncBackoff
	if backoff > maxSyncBackoff {
		return maxSyncBackoff
	}
	return backoff
}

func normalizeMeetingStatus(status string) domain.MeetingStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued", "created", "accepted":
		return domain.MeetingStatusUploaded
	case "processing", "running", "in_progress", "in-progress", "started":
		return domain.MeetingStatusProcessing
	case "completed", "done", "finished", "success", "succeeded":
		return domain.MeetingStatusCompleted
	case "failed", "error", "cancelled", "canceled", "timeout":
		return domain.MeetingStatusFailed
	default:
		return ""
	}
}

func toMeetingResponse(meeting *domain.Meeting) *MeetingResponse {
	if meeting == nil {
		return nil
	}
	return &MeetingResponse{
		ID:            meeting.ID,
		Title:         meeting.Title,
		Duration:      meeting.Duration,
		Status:        string(meeting.Status),
		WorkflowID:    meeting.WorkflowID,
		SyncFailCount: meeting.SyncFailCount,
		LastSyncError: meeting.LastSyncError,
		LastSyncAt:    meeting.LastSyncAt,
		NextSyncAt:    meeting.NextSyncAt,
		CreatedAt:     meeting.CreatedAt,
		UpdatedAt:     meeting.UpdatedAt,
	}
}

func (s *Service) publishMeetingUpdated(meeting *domain.Meeting) {
	if s == nil || s.eventPublisher == nil || meeting == nil || meeting.UserID == 0 {
		return
	}
	s.eventPublisher.PublishUserEvent(meeting.UserID, "meeting.updated", map[string]any{
		"meeting": toMeetingResponse(meeting),
	})
}

func buildSummaryInput(transcripts []domain.Transcript) string {
	lines := make([]string, 0, len(transcripts))
	for _, item := range transcripts {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		label := strings.TrimSpace(item.SpeakerLabel)
		if label != "" && label != "ASR" {
			lines = append(lines, fmt.Sprintf("%s：%s", label, text))
			continue
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

func extractMeetingTranscriptText(exec *appwf.ExecutionResponse, fallbackText string) string {
	fallback := strings.TrimSpace(fallbackText)
	if exec == nil {
		return fallback
	}

	for index := len(exec.NodeResults) - 1; index >= 0; index-- {
		result := exec.NodeResults[index]
		if result.NodeType != wfdomain.NodeMeetingSummary {
			continue
		}
		if result.Status == wfdomain.NodeResultSuccess {
			if text := strings.TrimSpace(result.InputText); text != "" {
				return text
			}
			for previous := index - 1; previous >= 0; previous-- {
				candidate := exec.NodeResults[previous]
				if candidate.Status != wfdomain.NodeResultSuccess {
					continue
				}
				if text := strings.TrimSpace(candidate.OutputText); text != "" {
					return text
				}
			}
		}
		if fallback != "" {
			return fallback
		}
		if text := strings.TrimSpace(exec.InputText); text != "" {
			return text
		}
		if text := strings.TrimSpace(exec.FinalText); text != "" {
			return text
		}
		return ""
	}

	if text := strings.TrimSpace(exec.FinalText); text != "" {
		return text
	}
	if fallback != "" {
		return fallback
	}
	return strings.TrimSpace(exec.InputText)
}

func extractMeetingSummary(exec *appwf.ExecutionResponse, workflowID uint64) (string, string, error) {
	if exec == nil {
		return "", "", fmt.Errorf("empty workflow execution result")
	}
	for index := len(exec.NodeResults) - 1; index >= 0; index-- {
		result := exec.NodeResults[index]
		if result.NodeType != wfdomain.NodeMeetingSummary {
			continue
		}
		if result.Status != wfdomain.NodeResultSuccess {
			return "", "", fmt.Errorf("meeting summary node did not complete successfully")
		}
		content := strings.TrimSpace(result.OutputText)
		if content == "" {
			return "", "", fmt.Errorf("meeting summary node returned empty output")
		}
		return content, extractSummaryModelVersion(result.Detail, workflowID), nil
	}
	return "", "", fmt.Errorf("selected workflow does not contain a meeting summary node")
}

func extractSummaryModelVersion(detail json.RawMessage, workflowID uint64) string {
	var payload struct {
		ModelVersion string `json:"model_version"`
		Model        string `json:"model"`
		Source       string `json:"source"`
	}
	if len(detail) > 0 {
		_ = json.Unmarshal(detail, &payload)
	}
	if payload.ModelVersion != "" {
		return payload.ModelVersion
	}
	if payload.Model != "" {
		return payload.Model
	}
	if payload.Source != "" {
		return payload.Source
	}
	return fmt.Sprintf("workflow:%d", workflowID)
}

func canDeleteMeeting(meeting *domain.Meeting) bool {
	if meeting == nil {
		return false
	}
	return meeting.Status != domain.MeetingStatusProcessing
}
