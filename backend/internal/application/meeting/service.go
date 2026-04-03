package meeting

import (
	"context"

	domain "github.com/lgt/asr/internal/domain/meeting"
)

// Service orchestrates meeting use cases.
type Service struct {
	meetingRepo    domain.MeetingRepository
	transcriptRepo domain.TranscriptRepository
	summaryRepo    domain.SummaryRepository
}

// NewService creates a new meeting application service.
func NewService(
	meetingRepo domain.MeetingRepository,
	transcriptRepo domain.TranscriptRepository,
	summaryRepo domain.SummaryRepository,
) *Service {
	return &Service{
		meetingRepo:    meetingRepo,
		transcriptRepo: transcriptRepo,
		summaryRepo:    summaryRepo,
	}
}

// CreateMeeting creates a new meeting record.
func (s *Service) CreateMeeting(ctx context.Context, userID uint64, req *CreateMeetingRequest) (*MeetingResponse, error) {
	m := &domain.Meeting{
		UserID:   userID,
		Title:    req.Title,
		AudioURL: req.AudioURL,
		Duration: req.Duration,
		Status:   domain.MeetingStatusUploaded,
	}
	if err := s.meetingRepo.Create(ctx, m); err != nil {
		return nil, err
	}
	return &MeetingResponse{
		ID:        m.ID,
		Title:     m.Title,
		Duration:  m.Duration,
		Status:    string(m.Status),
		CreatedAt: m.CreatedAt,
	}, nil
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
		MeetingResponse: MeetingResponse{
			ID:        m.ID,
			Title:     m.Title,
			Duration:  m.Duration,
			Status:    string(m.Status),
			CreatedAt: m.CreatedAt,
		},
		Transcripts: items,
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
		items[i] = &MeetingResponse{
			ID:        m.ID,
			Title:     m.Title,
			Duration:  m.Duration,
			Status:    string(m.Status),
			CreatedAt: m.CreatedAt,
		}
	}
	return items, total, nil
}
