package meeting

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	appwf "github.com/lgt/asr/internal/application/workflow"
	domain "github.com/lgt/asr/internal/domain/meeting"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"gorm.io/gorm"
)

type meetingRepoServiceStub struct {
	meeting *domain.Meeting
	updated *domain.Meeting
	created *domain.Meeting
	deleted uint64
}

func (s *meetingRepoServiceStub) Create(_ context.Context, meeting *domain.Meeting) error {
	copy := *meeting
	copy.ID = 101
	s.created = &copy
	if s.meeting == nil {
		s.meeting = &copy
	}
	meeting.ID = copy.ID
	meeting.CreatedAt = time.Now()
	meeting.UpdatedAt = meeting.CreatedAt
	return nil
}

func (s *meetingRepoServiceStub) GetByID(_ context.Context, id uint64) (*domain.Meeting, error) {
	if s.meeting == nil || s.meeting.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *s.meeting
	return &copy, nil
}

func (s *meetingRepoServiceStub) GetBySourceTaskID(_ context.Context, _ uint64) (*domain.Meeting, error) {
	panic("unexpected GetBySourceTaskID call")
}

func (s *meetingRepoServiceStub) Update(_ context.Context, meeting *domain.Meeting) error {
	copy := *meeting
	s.updated = &copy
	if s.meeting != nil {
		*s.meeting = copy
	}
	return nil
}

func (s *meetingRepoServiceStub) List(_ context.Context, _ uint64, _, _ int) ([]*domain.Meeting, int64, error) {
	panic("unexpected List call")
}

func (s *meetingRepoServiceStub) ListSyncCandidates(_ context.Context, _ int) ([]*domain.Meeting, error) {
	if s.meeting == nil {
		return nil, nil
	}
	copy := *s.meeting
	return []*domain.Meeting{&copy}, nil
}

func (s *meetingRepoServiceStub) Delete(_ context.Context, id uint64) error {
	s.deleted = id
	if s.meeting != nil && s.meeting.ID == id {
		s.meeting = nil
	}
	return nil
}

type transcriptRepoServiceStub struct {
	items            []domain.Transcript
	created          []domain.Transcript
	deletedMeetingID uint64
}

func (s *transcriptRepoServiceStub) BatchCreate(_ context.Context, transcripts []domain.Transcript) error {
	s.created = append([]domain.Transcript(nil), transcripts...)
	s.items = append([]domain.Transcript(nil), transcripts...)
	return nil
}

func (s *transcriptRepoServiceStub) ListByMeeting(_ context.Context, _ uint64) ([]domain.Transcript, error) {
	return s.items, nil
}

func (s *transcriptRepoServiceStub) DeleteByMeeting(_ context.Context, meetingID uint64) error {
	s.deletedMeetingID = meetingID
	s.items = nil
	return nil
}

type summaryRepoServiceStub struct {
	current          *domain.Summary
	created          *domain.Summary
	updated          *domain.Summary
	deletedMeetingID uint64
}

func (s *summaryRepoServiceStub) Create(_ context.Context, summary *domain.Summary) error {
	summary.ID = 11
	summary.CreatedAt = time.Now()
	copy := *summary
	s.created = &copy
	s.current = &copy
	return nil
}

func (s *summaryRepoServiceStub) GetByMeeting(_ context.Context, _ uint64) (*domain.Summary, error) {
	if s.current == nil {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *s.current
	return &copy, nil
}

func (s *summaryRepoServiceStub) Update(_ context.Context, summary *domain.Summary) error {
	copy := *summary
	s.updated = &copy
	s.current = &copy
	return nil
}

func (s *summaryRepoServiceStub) DeleteByMeeting(_ context.Context, meetingID uint64) error {
	s.deletedMeetingID = meetingID
	s.current = nil
	return nil
}

type workflowExecServiceStub struct {
	resp       *appwf.ExecutionResponse
	workflowID uint64
	inputText  string
	audioURL   string
	audioPath  string
}

func (s *workflowExecServiceStub) ExecuteMeetingSummaryWorkflow(_ context.Context, workflowID uint64, _ uint64, _ uint64, inputText, audioURL, audioFilePath string) (*appwf.ExecutionResponse, error) {
	s.workflowID = workflowID
	s.inputText = inputText
	s.audioURL = audioURL
	s.audioPath = audioFilePath
	if s.resp == nil {
		return nil, errors.New("missing response")
	}
	return s.resp, nil
}

type meetingBatchEngineStub struct {
	submitResult *BatchSubmitResult
	queryResult  *BatchTaskStatus
	submitCalls  int
	queryCalls   int
}

func (s *meetingBatchEngineStub) SubmitBatch(_ context.Context, _ BatchSubmitRequest) (*BatchSubmitResult, error) {
	s.submitCalls++
	if s.submitResult == nil {
		return nil, errors.New("missing submit result")
	}
	return s.submitResult, nil
}

func (s *meetingBatchEngineStub) QueryBatchTask(_ context.Context, _ string) (*BatchTaskStatus, error) {
	s.queryCalls++
	if s.queryResult == nil {
		return nil, errors.New("missing query result")
	}
	return s.queryResult, nil
}

func TestRegenerateSummaryPersistsWorkflowAndSummaryNodeOutput(t *testing.T) {
	workflowID := uint64(23)
	detail, err := json.Marshal(map[string]string{"model": "qwen-summary"})
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}

	meetingRepo := &meetingRepoServiceStub{meeting: &domain.Meeting{
		ID:            8,
		UserID:        5,
		Title:         "周会",
		AudioURL:      "https://example.com/meeting.wav",
		LocalFilePath: "/tmp/meeting.wav",
		Duration:      120,
		Status:        domain.MeetingStatusCompleted,
	}}
	transcriptRepo := &transcriptRepoServiceStub{items: []domain.Transcript{
		{SpeakerLabel: "Speaker A", Text: "第一段内容"},
		{SpeakerLabel: "Speaker B", Text: "第二段内容"},
	}}
	summaryRepo := &summaryRepoServiceStub{}
	workflowExec := &workflowExecServiceStub{resp: &appwf.ExecutionResponse{
		WorkflowID: workflowID,
		NodeResults: []appwf.NodeResultResponse{{
			NodeType:   wfdomain.NodeMeetingSummary,
			Status:     wfdomain.NodeResultSuccess,
			OutputText: "会议摘要内容",
			Detail:     detail,
		}},
	}}
	service := NewService(meetingRepo, transcriptRepo, summaryRepo, workflowExec, nil, nil)

	result, err := service.RegenerateSummary(context.Background(), 8, 5, &RegenerateSummaryRequest{WorkflowID: &workflowID})
	if err != nil {
		t.Fatalf("RegenerateSummary returned error: %v", err)
	}
	if meetingRepo.updated == nil || meetingRepo.updated.WorkflowID == nil || *meetingRepo.updated.WorkflowID != workflowID {
		t.Fatalf("expected meeting workflow to be updated to %d, got %+v", workflowID, meetingRepo.updated)
	}
	if workflowExec.workflowID != workflowID {
		t.Fatalf("expected workflow executor to receive workflow_id=%d, got %d", workflowID, workflowExec.workflowID)
	}
	if workflowExec.inputText != "Speaker A：第一段内容\nSpeaker B：第二段内容" {
		t.Fatalf("unexpected workflow input text: %q", workflowExec.inputText)
	}
	if workflowExec.audioURL != "https://example.com/meeting.wav" {
		t.Fatalf("expected audio url forwarded, got %q", workflowExec.audioURL)
	}
	if workflowExec.audioPath != "/tmp/meeting.wav" {
		t.Fatalf("expected local audio path forwarded, got %q", workflowExec.audioPath)
	}
	if summaryRepo.created == nil || summaryRepo.created.Content != "会议摘要内容" {
		t.Fatalf("expected summary to be created from meeting_summary node output, got %+v", summaryRepo.created)
	}
	if summaryRepo.created.ModelVersion != "qwen-summary" {
		t.Fatalf("expected summary model version from node detail, got %q", summaryRepo.created.ModelVersion)
	}
	if result.WorkflowID == nil || *result.WorkflowID != workflowID {
		t.Fatalf("expected response workflow_id=%d, got %+v", workflowID, result.WorkflowID)
	}
	if result.Summary == nil || result.Summary.Content != "会议摘要内容" {
		t.Fatalf("expected response summary to be refreshed, got %+v", result.Summary)
	}
}

func TestCreateMeetingDefaultsTitleFromAudioSource(t *testing.T) {
	meetingRepo := &meetingRepoServiceStub{}
	service := NewService(meetingRepo, nil, nil, nil, nil, nil)

	result, err := service.CreateMeeting(context.Background(), 9, &CreateMeetingRequest{
		AudioURL: "https://example.com/meeting.wav",
	})
	if err != nil {
		t.Fatalf("CreateMeeting returned error: %v", err)
	}
	if meetingRepo.created == nil {
		t.Fatal("expected meeting to be created")
	}
	wantTitle := "meeting"
	if meetingRepo.created.Title != wantTitle {
		t.Fatalf("expected default title %q, got %q", wantTitle, meetingRepo.created.Title)
	}
	if result.Title != wantTitle {
		t.Fatalf("expected response title %q, got %q", wantTitle, result.Title)
	}
}

func TestDeleteMeetingRemovesRelatedData(t *testing.T) {
	meetingRepo := &meetingRepoServiceStub{meeting: &domain.Meeting{
		ID:       11,
		UserID:   9,
		Status:   domain.MeetingStatusCompleted,
		Title:    "会诊",
		AudioURL: "https://example.com/a.wav",
	}}
	transcriptRepo := &transcriptRepoServiceStub{items: []domain.Transcript{{MeetingID: 11, Text: "逐字稿"}}}
	summaryRepo := &summaryRepoServiceStub{current: &domain.Summary{MeetingID: 11, Content: "摘要"}}
	service := NewService(meetingRepo, transcriptRepo, summaryRepo, nil, nil, nil)

	if err := service.DeleteMeeting(context.Background(), 11, 9); err != nil {
		t.Fatalf("DeleteMeeting returned error: %v", err)
	}
	if meetingRepo.deleted != 11 {
		t.Fatalf("expected meeting 11 deleted, got %d", meetingRepo.deleted)
	}
	if transcriptRepo.deletedMeetingID != 11 {
		t.Fatalf("expected transcript delete for meeting 11, got %d", transcriptRepo.deletedMeetingID)
	}
	if summaryRepo.deletedMeetingID != 11 {
		t.Fatalf("expected summary delete for meeting 11, got %d", summaryRepo.deletedMeetingID)
	}
}

func TestDeleteMeetingRejectsProcessingMeeting(t *testing.T) {
	meetingRepo := &meetingRepoServiceStub{meeting: &domain.Meeting{
		ID:       12,
		UserID:   9,
		Status:   domain.MeetingStatusProcessing,
		Title:    "会诊",
		AudioURL: "https://example.com/a.wav",
	}}
	service := NewService(meetingRepo, &transcriptRepoServiceStub{}, &summaryRepoServiceStub{}, nil, nil, nil)

	err := service.DeleteMeeting(context.Background(), 12, 9)
	if !errors.Is(err, ErrMeetingDeleteNotAllowed) {
		t.Fatalf("expected ErrMeetingDeleteNotAllowed, got %v", err)
	}
}

func TestSyncPendingMeetingsCompletesMeetingIndependently(t *testing.T) {
	workflowID := uint64(31)
	detail, err := json.Marshal(map[string]string{"model_version": "meeting-summary-v2"})
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}

	meetingRepo := &meetingRepoServiceStub{meeting: &domain.Meeting{
		ID:         15,
		UserID:     9,
		Title:      "季度复盘",
		AudioURL:   "https://example.com/review.wav",
		WorkflowID: &workflowID,
		Status:     domain.MeetingStatusUploaded,
	}}
	transcriptRepo := &transcriptRepoServiceStub{}
	summaryRepo := &summaryRepoServiceStub{}
	workflowExec := &workflowExecServiceStub{resp: &appwf.ExecutionResponse{
		WorkflowID: workflowID,
		FinalText:  "最终会议摘要",
		NodeResults: []appwf.NodeResultResponse{{
			NodeType:   wfdomain.NodeMeetingSummary,
			Status:     wfdomain.NodeResultSuccess,
			InputText:  "整理后的逐字稿",
			OutputText: "最终会议摘要",
			Detail:     detail,
		}},
	}}
	batchEngine := &meetingBatchEngineStub{submitResult: &BatchSubmitResult{
		Status:     "completed",
		ResultText: "原始逐字稿",
		Duration:   180,
	}}
	service := NewService(meetingRepo, transcriptRepo, summaryRepo, workflowExec, batchEngine, nil)

	summary, err := service.SyncPendingMeetings(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncPendingMeetings returned error: %v", err)
	}
	if summary.Scanned != 1 || summary.Updated != 1 || summary.Failed != 0 {
		t.Fatalf("unexpected sync summary: %+v", summary)
	}
	if batchEngine.submitCalls != 1 {
		t.Fatalf("expected one submit call, got %d", batchEngine.submitCalls)
	}
	if meetingRepo.meeting == nil || meetingRepo.meeting.Status != domain.MeetingStatusCompleted {
		t.Fatalf("expected meeting completed, got %+v", meetingRepo.meeting)
	}
	if meetingRepo.meeting.Duration != 180 {
		t.Fatalf("expected duration 180, got %v", meetingRepo.meeting.Duration)
	}
	if len(transcriptRepo.created) != 1 || transcriptRepo.created[0].Text != "整理后的逐字稿" {
		t.Fatalf("expected processed transcript persisted, got %+v", transcriptRepo.created)
	}
	if summaryRepo.created == nil || summaryRepo.created.Content != "最终会议摘要" {
		t.Fatalf("expected summary created, got %+v", summaryRepo.created)
	}
	if workflowExec.inputText != "原始逐字稿" {
		t.Fatalf("expected workflow input to use ASR result, got %q", workflowExec.inputText)
	}
}

func TestSyncPendingMeetingsFallsBackToPreSummaryNodeOutput(t *testing.T) {
	workflowID := uint64(32)
	detail, err := json.Marshal(map[string]string{"model_version": "meeting-summary-v2"})
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}

	meetingRepo := &meetingRepoServiceStub{meeting: &domain.Meeting{
		ID:         16,
		UserID:     9,
		Title:      "病例讨论",
		AudioURL:   "https://example.com/case.wav",
		WorkflowID: &workflowID,
		Status:     domain.MeetingStatusUploaded,
	}}
	transcriptRepo := &transcriptRepoServiceStub{}
	summaryRepo := &summaryRepoServiceStub{}
	workflowExec := &workflowExecServiceStub{resp: &appwf.ExecutionResponse{
		WorkflowID: workflowID,
		FinalText:  "最终会议摘要",
		NodeResults: []appwf.NodeResultResponse{
			{
				NodeType:   wfdomain.NodeLLMCorrection,
				Status:     wfdomain.NodeResultSuccess,
				OutputText: "前置清洗后的逐字稿",
			},
			{
				NodeType:   wfdomain.NodeMeetingSummary,
				Status:     wfdomain.NodeResultSuccess,
				OutputText: "最终会议摘要",
				Detail:     detail,
			},
		},
	}}
	batchEngine := &meetingBatchEngineStub{submitResult: &BatchSubmitResult{
		Status:     "completed",
		ResultText: "原始逐字稿",
		Duration:   96,
	}}
	service := NewService(meetingRepo, transcriptRepo, summaryRepo, workflowExec, batchEngine, nil)

	summary, err := service.SyncPendingMeetings(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncPendingMeetings returned error: %v", err)
	}
	if summary.Scanned != 1 || summary.Updated != 1 || summary.Failed != 0 {
		t.Fatalf("unexpected sync summary: %+v", summary)
	}
	if len(transcriptRepo.created) != 1 || transcriptRepo.created[0].Text != "前置清洗后的逐字稿" {
		t.Fatalf("expected transcript to fall back to pre-summary output, got %+v", transcriptRepo.created)
	}
	if summaryRepo.created == nil || summaryRepo.created.Content != "最终会议摘要" {
		t.Fatalf("expected summary created, got %+v", summaryRepo.created)
	}
}

func TestSyncPendingMeetingsQueriesExistingExternalTask(t *testing.T) {
	meetingRepo := &meetingRepoServiceStub{meeting: &domain.Meeting{
		ID:             19,
		UserID:         5,
		Title:          "周例会",
		AudioURL:       "https://example.com/weekly.wav",
		ExternalTaskID: "remote-task-1",
		Status:         domain.MeetingStatusProcessing,
	}}
	transcriptRepo := &transcriptRepoServiceStub{}
	batchEngine := &meetingBatchEngineStub{queryResult: &BatchTaskStatus{
		Status:     "completed",
		ResultText: "查询得到的逐字稿",
		Duration:   66,
	}}
	service := NewService(meetingRepo, transcriptRepo, &summaryRepoServiceStub{}, nil, batchEngine, nil)

	summary, err := service.SyncPendingMeetings(context.Background(), 10)
	if err != nil {
		t.Fatalf("SyncPendingMeetings returned error: %v", err)
	}
	if summary.Scanned != 1 || summary.Updated != 1 || summary.Failed != 0 {
		t.Fatalf("unexpected sync summary: %+v", summary)
	}
	if batchEngine.queryCalls != 1 {
		t.Fatalf("expected one query call, got %d", batchEngine.queryCalls)
	}
	if meetingRepo.meeting == nil || meetingRepo.meeting.Status != domain.MeetingStatusCompleted {
		t.Fatalf("expected meeting completed, got %+v", meetingRepo.meeting)
	}
	if len(transcriptRepo.created) != 1 || transcriptRepo.created[0].Text != "查询得到的逐字稿" {
		t.Fatalf("expected queried transcript persisted, got %+v", transcriptRepo.created)
	}
}
