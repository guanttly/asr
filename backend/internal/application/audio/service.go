package audio

import (
	"context"
	"fmt"

	appasr "github.com/lgt/asr/internal/application/asr"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	domainasr "github.com/lgt/asr/internal/domain/asr"
)

type ASRService interface {
	CreateTask(ctx context.Context, userID uint64, req *appasr.CreateTaskRequest) (*appasr.TaskResponse, error)
	TranscribeSnippet(ctx context.Context, req *appasr.TranscribeSnippetRequest) (*appasr.TranscribeSnippetResponse, error)
}

type MeetingService interface {
	CreateMeeting(ctx context.Context, userID uint64, req *appmeeting.CreateMeetingRequest) (*appmeeting.MeetingResponse, error)
}

type Service struct {
	asrService     ASRService
	meetingService MeetingService
}

type PreparedAudio struct {
	OriginalFilename string
	AudioURL         string
	LocalFilePath    string
	Duration         float64
}

type CreateBatchTaskRequest struct {
	Audio      PreparedAudio
	DictID     *uint64
	WorkflowID *uint64
}

type CreateMeetingRequest struct {
	Audio      PreparedAudio
	Title      string
	WorkflowID *uint64
}

type CreateRealtimeTaskRequest struct {
	Audio      PreparedAudio
	ResultText string
	WorkflowID *uint64
}

type TranscribeRealtimeSegmentRequest struct {
	Audio  PreparedAudio
	DictID *uint64
}

func NewService(asrService ASRService, meetingService MeetingService) *Service {
	return &Service{asrService: asrService, meetingService: meetingService}
}

func (s *Service) CreateBatchTaskFromAudio(ctx context.Context, userID uint64, req CreateBatchTaskRequest) (*appasr.TaskResponse, error) {
	if s.asrService == nil {
		return nil, fmt.Errorf("asr service unavailable")
	}

	return s.asrService.CreateTask(ctx, userID, &appasr.CreateTaskRequest{
		AudioURL:      req.Audio.AudioURL,
		LocalFilePath: req.Audio.LocalFilePath,
		Type:          domainasr.TaskTypeBatch,
		DictID:        req.DictID,
		WorkflowID:    req.WorkflowID,
		Duration:      req.Audio.Duration,
	})
}

func (s *Service) CreateRealtimeTaskFromAudio(ctx context.Context, userID uint64, req CreateRealtimeTaskRequest) (*appasr.TaskResponse, error) {
	if s.asrService == nil {
		return nil, fmt.Errorf("asr service unavailable")
	}

	return s.asrService.CreateTask(ctx, userID, &appasr.CreateTaskRequest{
		AudioURL:      req.Audio.AudioURL,
		LocalFilePath: req.Audio.LocalFilePath,
		Type:          domainasr.TaskTypeRealtime,
		WorkflowID:    req.WorkflowID,
		ResultText:    req.ResultText,
		Duration:      req.Audio.Duration,
	})
}

func (s *Service) CreateMeetingFromAudio(ctx context.Context, userID uint64, req CreateMeetingRequest) (*appmeeting.MeetingResponse, error) {
	if s.meetingService == nil {
		return nil, fmt.Errorf("meeting service unavailable")
	}

	return s.meetingService.CreateMeeting(ctx, userID, &appmeeting.CreateMeetingRequest{
		Title:         req.Title,
		AudioURL:      req.Audio.AudioURL,
		LocalFilePath: req.Audio.LocalFilePath,
		Duration:      req.Audio.Duration,
		WorkflowID:    req.WorkflowID,
	})
}

func (s *Service) TranscribeRealtimeSegment(ctx context.Context, req TranscribeRealtimeSegmentRequest) (*appasr.TranscribeSnippetResponse, error) {
	if s.asrService == nil {
		return nil, fmt.Errorf("asr service unavailable")
	}

	return s.asrService.TranscribeSnippet(ctx, &appasr.TranscribeSnippetRequest{
		LocalFilePath: req.Audio.LocalFilePath,
		DictID:        req.DictID,
	})
}
