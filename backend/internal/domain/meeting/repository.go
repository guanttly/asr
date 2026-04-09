package meeting

import "context"

// MeetingRepository defines persistence operations for Meeting aggregate.
type MeetingRepository interface {
	Create(ctx context.Context, m *Meeting) error
	GetByID(ctx context.Context, id uint64) (*Meeting, error)
	GetBySourceTaskID(ctx context.Context, sourceTaskID uint64) (*Meeting, error)
	Update(ctx context.Context, m *Meeting) error
	List(ctx context.Context, userID uint64, offset, limit int) ([]*Meeting, int64, error)
	ListSyncCandidates(ctx context.Context, limit int) ([]*Meeting, error)
	Delete(ctx context.Context, id uint64) error
}

// TranscriptRepository manages meeting transcript segments.
type TranscriptRepository interface {
	BatchCreate(ctx context.Context, transcripts []Transcript) error
	ListByMeeting(ctx context.Context, meetingID uint64) ([]Transcript, error)
	DeleteByMeeting(ctx context.Context, meetingID uint64) error
}

// SummaryRepository manages meeting summaries.
type SummaryRepository interface {
	Create(ctx context.Context, s *Summary) error
	GetByMeeting(ctx context.Context, meetingID uint64) (*Summary, error)
	Update(ctx context.Context, s *Summary) error
	DeleteByMeeting(ctx context.Context, meetingID uint64) error
}
