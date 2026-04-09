package postprocess

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	appwf "github.com/lgt/asr/internal/application/workflow"
	asrdomain "github.com/lgt/asr/internal/domain/asr"
	meetingdomain "github.com/lgt/asr/internal/domain/meeting"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
)

type workflowExecutorStub struct {
	resp *appwf.ExecutionResponse
	err  error
}

func (s *workflowExecutorStub) ExecuteForTask(_ context.Context, _ *asrdomain.TranscriptionTask, _ string) (*appwf.ExecutionResponse, error) {
	return s.resp, s.err
}

func (s *workflowExecutorStub) ResumeForTaskFromFailure(_ context.Context, _ *asrdomain.TranscriptionTask) (*appwf.ExecutionResponse, error) {
	return s.resp, s.err
}

type meetingRepoStub struct {
	created *meetingdomain.Meeting
}

func (s *meetingRepoStub) Create(_ context.Context, meeting *meetingdomain.Meeting) error {
	meeting.ID = 101
	s.created = meeting
	return nil
}

func (s *meetingRepoStub) GetByID(_ context.Context, _ uint64) (*meetingdomain.Meeting, error) {
	panic("unexpected GetByID call")
}

func (s *meetingRepoStub) GetBySourceTaskID(_ context.Context, _ uint64) (*meetingdomain.Meeting, error) {
	return nil, nil
}

func (s *meetingRepoStub) Update(_ context.Context, _ *meetingdomain.Meeting) error {
	panic("unexpected Update call")
}

func (s *meetingRepoStub) List(_ context.Context, _ uint64, _, _ int) ([]*meetingdomain.Meeting, int64, error) {
	panic("unexpected List call")
}

func (s *meetingRepoStub) ListSyncCandidates(_ context.Context, _ int) ([]*meetingdomain.Meeting, error) {
	panic("unexpected ListSyncCandidates call")
}

func (s *meetingRepoStub) Delete(_ context.Context, _ uint64) error {
	panic("unexpected Delete call")
}

type transcriptRepoStub struct {
	created []meetingdomain.Transcript
}

func (s *transcriptRepoStub) BatchCreate(_ context.Context, transcripts []meetingdomain.Transcript) error {
	s.created = transcripts
	return nil
}

func (s *transcriptRepoStub) ListByMeeting(_ context.Context, _ uint64) ([]meetingdomain.Transcript, error) {
	panic("unexpected ListByMeeting call")
}

func (s *transcriptRepoStub) DeleteByMeeting(_ context.Context, _ uint64) error {
	panic("unexpected DeleteByMeeting call")
}

type summaryRepoStub struct {
	created *meetingdomain.Summary
}

func (s *summaryRepoStub) Create(_ context.Context, summary *meetingdomain.Summary) error {
	s.created = summary
	return nil
}

func (s *summaryRepoStub) GetByMeeting(_ context.Context, _ uint64) (*meetingdomain.Summary, error) {
	panic("unexpected GetByMeeting call")
}

func (s *summaryRepoStub) Update(_ context.Context, _ *meetingdomain.Summary) error {
	panic("unexpected Update call")
}

func (s *summaryRepoStub) DeleteByMeeting(_ context.Context, _ uint64) error {
	panic("unexpected DeleteByMeeting call")
}

func TestWorkflowAwareProcessorPersistsMeetingSummaryNodeOutput(t *testing.T) {
	workflowID := uint64(9)
	detail, err := json.Marshal(map[string]string{"model_version": "summary-v1"})
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}

	meetingRepo := &meetingRepoStub{}
	transcriptRepo := &transcriptRepoStub{}
	summaryRepo := &summaryRepoStub{}
	processor := NewWorkflowAwareProcessor(
		nil,
		&workflowExecutorStub{resp: &appwf.ExecutionResponse{
			WorkflowID: workflowID,
			FinalText:  "整理后的逐字稿",
			NodeResults: []appwf.NodeResultResponse{{
				NodeType:   wfdomain.NodeMeetingSummary,
				Status:     wfdomain.NodeResultSuccess,
				OutputText: "这是会议摘要",
				Detail:     detail,
			}},
		}},
		meetingRepo,
		transcriptRepo,
		summaryRepo,
	)

	task := &asrdomain.TranscriptionTask{
		ID:         77,
		UserID:     3,
		AudioURL:   "https://example.com/audio.wav",
		Duration:   35,
		ResultText: "原始逐字稿",
		WorkflowID: &workflowID,
	}

	if err := processor.ProcessCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessCompletedTask returned error: %v", err)
	}
	if meetingRepo.created == nil || meetingRepo.created.WorkflowID == nil || *meetingRepo.created.WorkflowID != workflowID {
		t.Fatalf("expected created meeting to retain workflow_id=%d, got %+v", workflowID, meetingRepo.created)
	}
	if len(transcriptRepo.created) != 1 || transcriptRepo.created[0].Text != "整理后的逐字稿" {
		t.Fatalf("expected processed transcript to be persisted, got %+v", transcriptRepo.created)
	}
	if summaryRepo.created == nil {
		t.Fatal("expected summary to be created")
	}
	if summaryRepo.created.Content != "这是会议摘要" {
		t.Fatalf("expected summary content to match meeting_summary node output, got %q", summaryRepo.created.Content)
	}
	if summaryRepo.created.ModelVersion != "summary-v1" {
		t.Fatalf("expected summary model version to be extracted from node detail, got %q", summaryRepo.created.ModelVersion)
	}
}

func TestWorkflowAwareProcessorSkipsMeetingMaterializationWithoutMeetingSummaryNode(t *testing.T) {
	workflowID := uint64(15)
	meetingRepo := &meetingRepoStub{}
	transcriptRepo := &transcriptRepoStub{}
	summaryRepo := &summaryRepoStub{}
	processor := NewWorkflowAwareProcessor(
		nil,
		&workflowExecutorStub{resp: &appwf.ExecutionResponse{
			WorkflowID: workflowID,
			FinalText:  "整理后的普通转写文本",
			NodeResults: []appwf.NodeResultResponse{{
				NodeType:   wfdomain.NodeFillerFilter,
				Status:     wfdomain.NodeResultSuccess,
				OutputText: "整理后的普通转写文本",
			}},
		}},
		meetingRepo,
		transcriptRepo,
		summaryRepo,
	)

	task := &asrdomain.TranscriptionTask{
		ID:         88,
		UserID:     4,
		AudioURL:   "https://example.com/audio.wav",
		Duration:   20,
		ResultText: "原始文本",
		WorkflowID: &workflowID,
	}

	if err := processor.ProcessCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessCompletedTask returned error: %v", err)
	}
	if meetingRepo.created != nil {
		t.Fatalf("expected no meeting to be created, got %+v", meetingRepo.created)
	}
	if len(transcriptRepo.created) != 0 {
		t.Fatalf("expected no transcript rows to be created, got %+v", transcriptRepo.created)
	}
	if summaryRepo.created != nil {
		t.Fatalf("expected no summary to be created, got %+v", summaryRepo.created)
	}
	if task.MeetingID != nil {
		t.Fatalf("expected task meeting_id to remain nil, got %+v", task.MeetingID)
	}
	if task.ResultText != "整理后的普通转写文本" {
		t.Fatalf("expected task result text to be updated, got %q", task.ResultText)
	}
}

func TestWorkflowAwareProcessorPreservesPartialFinalTextOnFailure(t *testing.T) {
	workflowID := uint64(21)
	processor := NewWorkflowAwareProcessor(
		&BatchMeetingProcessor{},
		&workflowExecutorStub{
			resp: &appwf.ExecutionResponse{
				WorkflowID: workflowID,
				FinalText:  "已经过滤掉语气词的文本",
			},
			err: context.DeadlineExceeded,
		},
		nil,
		nil,
		nil,
	)

	task := &asrdomain.TranscriptionTask{
		ID:         66,
		UserID:     3,
		Type:       asrdomain.TaskTypeBatch,
		ResultText: "嗯 原始文本",
		WorkflowID: &workflowID,
	}

	err := processor.ProcessCompletedTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected workflow failure")
	}
	if task.ResultText != "已经过滤掉语气词的文本" {
		t.Fatalf("expected partial workflow output to remain visible, got %q", task.ResultText)
	}
}

func TestWorkflowAwareProcessorMaterializesSpeakerSegmentsIntoMeetingTranscripts(t *testing.T) {
	workflowID := uint64(31)
	speakerDetail, err := json.Marshal(map[string]any{
		"segments_count": 2,
		"segments": []map[string]any{
			{"speaker": "Speaker A", "start_time": 0, "end_time": 12.5},
			{"speaker": "Speaker B", "start_time": 12.5, "end_time": 25},
		},
	})
	if err != nil {
		t.Fatalf("marshal speaker detail: %v", err)
	}
	summaryDetail, err := json.Marshal(map[string]string{"model_version": "summary-v2"})
	if err != nil {
		t.Fatalf("marshal summary detail: %v", err)
	}

	meetingRepo := &meetingRepoStub{}
	transcriptRepo := &transcriptRepoStub{}
	summaryRepo := &summaryRepoStub{}
	processor := NewWorkflowAwareProcessor(
		nil,
		&workflowExecutorStub{resp: &appwf.ExecutionResponse{
			WorkflowID: workflowID,
			FinalText:  "Speaker A：第一部分内容。Speaker B：第二部分内容。",
			NodeResults: []appwf.NodeResultResponse{
				{
					NodeType:   wfdomain.NodeSpeakerDiarize,
					Status:     wfdomain.NodeResultSuccess,
					OutputText: "[Speaker A 0.0s-12.5s]\n[Speaker B 12.5s-25.0s]\n\n第一部分内容。第二部分内容。",
					Detail:     speakerDetail,
				},
				{
					NodeType:   wfdomain.NodeMeetingSummary,
					Status:     wfdomain.NodeResultSuccess,
					InputText:  "第一部分内容。第二部分内容。",
					OutputText: "摘要内容",
					Detail:     summaryDetail,
				},
			},
		}},
		meetingRepo,
		transcriptRepo,
		summaryRepo,
	)

	task := &asrdomain.TranscriptionTask{
		ID:         100,
		UserID:     6,
		AudioURL:   "https://example.com/lesson.wav",
		Duration:   25,
		ResultText: "原始逐字稿",
		WorkflowID: &workflowID,
	}

	if err := processor.ProcessCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessCompletedTask returned error: %v", err)
	}
	if len(transcriptRepo.created) != 2 {
		t.Fatalf("expected 2 transcript rows, got %+v", transcriptRepo.created)
	}
	if transcriptRepo.created[0].SpeakerLabel != "Speaker A" || transcriptRepo.created[1].SpeakerLabel != "Speaker B" {
		t.Fatalf("expected speaker labels to come from diarization, got %+v", transcriptRepo.created)
	}
	if transcriptRepo.created[0].StartTime != 0 || transcriptRepo.created[0].EndTime != 12.5 {
		t.Fatalf("expected first segment timing preserved, got %+v", transcriptRepo.created[0])
	}
	if transcriptRepo.created[1].StartTime != 12.5 || transcriptRepo.created[1].EndTime != 25 {
		t.Fatalf("expected second segment timing preserved, got %+v", transcriptRepo.created[1])
	}
	if strings.TrimSpace(transcriptRepo.created[0].Text) == "" || strings.TrimSpace(transcriptRepo.created[1].Text) == "" {
		t.Fatalf("expected transcript texts to be split across segments, got %+v", transcriptRepo.created)
	}
	if summaryRepo.created == nil || summaryRepo.created.Content != "摘要内容" {
		t.Fatalf("expected summary persisted, got %+v", summaryRepo.created)
	}
}
