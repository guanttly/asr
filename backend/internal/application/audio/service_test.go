package audio

import (
	"context"
	"errors"
	"testing"

	appasr "github.com/lgt/asr/internal/application/asr"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	domainasr "github.com/lgt/asr/internal/domain/asr"
)

type asrServiceStub struct {
	createTaskResp   *appasr.TaskResponse
	transcribeResp   *appasr.TranscribeSnippetResponse
	createTaskErr    error
	transcribeErr    error
	lastCreateUserID uint64
	lastCreateReq    *appasr.CreateTaskRequest
	lastSnippetReq   *appasr.TranscribeSnippetRequest
}

func (s *asrServiceStub) CreateTask(_ context.Context, userID uint64, req *appasr.CreateTaskRequest) (*appasr.TaskResponse, error) {
	s.lastCreateUserID = userID
	if req != nil {
		copyReq := *req
		s.lastCreateReq = &copyReq
	}
	if s.createTaskErr != nil {
		return nil, s.createTaskErr
	}
	return s.createTaskResp, nil
}

func (s *asrServiceStub) TranscribeSnippet(_ context.Context, req *appasr.TranscribeSnippetRequest) (*appasr.TranscribeSnippetResponse, error) {
	if req != nil {
		copyReq := *req
		s.lastSnippetReq = &copyReq
	}
	if s.transcribeErr != nil {
		return nil, s.transcribeErr
	}
	return s.transcribeResp, nil
}

type meetingServiceStub struct {
	resp       *appmeeting.MeetingResponse
	err        error
	lastUserID uint64
	lastReq    *appmeeting.CreateMeetingRequest
}

func (s *meetingServiceStub) CreateMeeting(_ context.Context, userID uint64, req *appmeeting.CreateMeetingRequest) (*appmeeting.MeetingResponse, error) {
	s.lastUserID = userID
	if req != nil {
		copyReq := *req
		s.lastReq = &copyReq
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

func TestCreateBatchTaskFromAudioMapsRequest(t *testing.T) {
	asrStub := &asrServiceStub{createTaskResp: &appasr.TaskResponse{ID: 12}}
	svc := NewService(asrStub, nil)

	workflowID := uint64(8)
	dictID := uint64(3)
	resp, err := svc.CreateBatchTaskFromAudio(context.Background(), 7, CreateBatchTaskRequest{
		Audio: PreparedAudio{
			AudioURL:      "https://example.com/audio.wav",
			LocalFilePath: "/tmp/audio.wav",
			Duration:      42.5,
		},
		DictID:     &dictID,
		WorkflowID: &workflowID,
	})
	if err != nil {
		t.Fatalf("CreateBatchTaskFromAudio returned error: %v", err)
	}
	if resp == nil || resp.ID != 12 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if asrStub.lastCreateUserID != 7 {
		t.Fatalf("expected user id 7, got %d", asrStub.lastCreateUserID)
	}
	if asrStub.lastCreateReq == nil {
		t.Fatal("expected create task request to be forwarded")
	}
	if asrStub.lastCreateReq.Type != domainasr.TaskTypeBatch {
		t.Fatalf("expected batch task type, got %q", asrStub.lastCreateReq.Type)
	}
	if asrStub.lastCreateReq.AudioURL != "https://example.com/audio.wav" {
		t.Fatalf("unexpected audio url: %s", asrStub.lastCreateReq.AudioURL)
	}
	if asrStub.lastCreateReq.LocalFilePath != "/tmp/audio.wav" {
		t.Fatalf("unexpected local file path: %s", asrStub.lastCreateReq.LocalFilePath)
	}
	if asrStub.lastCreateReq.Duration != 42.5 {
		t.Fatalf("unexpected duration: %v", asrStub.lastCreateReq.Duration)
	}
}

func TestCreateBatchTaskFromAudioRequiresASRService(t *testing.T) {
	svc := NewService(nil, nil)
	_, err := svc.CreateBatchTaskFromAudio(context.Background(), 1, CreateBatchTaskRequest{})
	if err == nil || err.Error() != "asr service unavailable" {
		t.Fatalf("expected asr service unavailable error, got %v", err)
	}
}

func TestCreateRealtimeTaskFromAudioMapsRequest(t *testing.T) {
	asrStub := &asrServiceStub{createTaskResp: &appasr.TaskResponse{ID: 18}}
	svc := NewService(asrStub, nil)

	workflowID := uint64(10)
	resp, err := svc.CreateRealtimeTaskFromAudio(context.Background(), 7, CreateRealtimeTaskRequest{
		Audio: PreparedAudio{
			AudioURL:      "https://example.com/realtime.wav",
			LocalFilePath: "/tmp/realtime.wav",
			Duration:      12.3,
		},
		ResultText: "整段实时文本",
		WorkflowID: &workflowID,
	})
	if err != nil {
		t.Fatalf("CreateRealtimeTaskFromAudio returned error: %v", err)
	}
	if resp == nil || resp.ID != 18 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if asrStub.lastCreateReq == nil {
		t.Fatal("expected realtime create task request to be forwarded")
	}
	if asrStub.lastCreateReq.Type != domainasr.TaskTypeRealtime {
		t.Fatalf("expected realtime task type, got %q", asrStub.lastCreateReq.Type)
	}
	if asrStub.lastCreateReq.ResultText != "整段实时文本" {
		t.Fatalf("unexpected result text: %s", asrStub.lastCreateReq.ResultText)
	}
	if asrStub.lastCreateReq.AudioURL != "https://example.com/realtime.wav" {
		t.Fatalf("unexpected audio url: %s", asrStub.lastCreateReq.AudioURL)
	}
	if asrStub.lastCreateReq.LocalFilePath != "/tmp/realtime.wav" {
		t.Fatalf("unexpected local file path: %s", asrStub.lastCreateReq.LocalFilePath)
	}
	if asrStub.lastCreateReq.Duration != 12.3 {
		t.Fatalf("unexpected duration: %v", asrStub.lastCreateReq.Duration)
	}
}

func TestTranscribeRealtimeSegmentMapsRequest(t *testing.T) {
	asrStub := &asrServiceStub{transcribeResp: &appasr.TranscribeSnippetResponse{Text: "ok"}}
	svc := NewService(asrStub, nil)

	resp, err := svc.TranscribeRealtimeSegment(context.Background(), TranscribeRealtimeSegmentRequest{
		Audio: PreparedAudio{LocalFilePath: "/tmp/realtime.wav", Duration: 1.2},
	})
	if err != nil {
		t.Fatalf("TranscribeRealtimeSegment returned error: %v", err)
	}
	if resp == nil || resp.Text != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if asrStub.lastSnippetReq == nil || asrStub.lastSnippetReq.LocalFilePath != "/tmp/realtime.wav" {
		t.Fatalf("unexpected snippet request: %+v", asrStub.lastSnippetReq)
	}
}

func TestCreateMeetingFromAudioMapsRequest(t *testing.T) {
	meetingStub := &meetingServiceStub{resp: &appmeeting.MeetingResponse{ID: 5}}
	svc := NewService(nil, meetingStub)

	workflowID := uint64(11)
	resp, err := svc.CreateMeetingFromAudio(context.Background(), 9, CreateMeetingRequest{
		Audio: PreparedAudio{
			AudioURL:      "https://example.com/meeting.wav",
			LocalFilePath: "/tmp/meeting.wav",
			Duration:      88.8,
		},
		Title:      "周会",
		WorkflowID: &workflowID,
	})
	if err != nil {
		t.Fatalf("CreateMeetingFromAudio returned error: %v", err)
	}
	if resp == nil || resp.ID != 5 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if meetingStub.lastUserID != 9 {
		t.Fatalf("expected user id 9, got %d", meetingStub.lastUserID)
	}
	if meetingStub.lastReq == nil {
		t.Fatal("expected meeting create request to be forwarded")
	}
	if meetingStub.lastReq.Title != "周会" {
		t.Fatalf("unexpected title: %s", meetingStub.lastReq.Title)
	}
	if meetingStub.lastReq.Duration != 88.8 {
		t.Fatalf("unexpected duration: %v", meetingStub.lastReq.Duration)
	}
}

func TestCreateMeetingFromAudioPropagatesError(t *testing.T) {
	meetingStub := &meetingServiceStub{err: errors.New("boom")}
	svc := NewService(nil, meetingStub)
	_, err := svc.CreateMeetingFromAudio(context.Background(), 1, CreateMeetingRequest{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected propagated error, got %v", err)
	}
}
