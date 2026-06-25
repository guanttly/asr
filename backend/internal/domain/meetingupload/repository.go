package meetingupload

import (
	"context"
	"time"
)

// Repository persists upload sessions and their segments.
type Repository interface {
	CreateSession(ctx context.Context, session *UploadSession) error
	GetSessionByUploadID(ctx context.Context, uploadID string) (*UploadSession, error)
	GetSessionByID(ctx context.Context, id uint64) (*UploadSession, error)
	UpdateSession(ctx context.Context, session *UploadSession) error
	DeleteSession(ctx context.Context, id uint64) error

	CreateSegment(ctx context.Context, segment *UploadSegment) error
	GetSegment(ctx context.Context, sessionID uint64, index int) (*UploadSegment, error)
	ListSegments(ctx context.Context, sessionID uint64) ([]*UploadSegment, error)
	DeleteSegments(ctx context.Context, sessionID uint64) error

	// ListStaleRecording returns sessions still marked recording whose
	// last_seen_at is older than the cutoff (lost heartbeat).
	ListStaleRecording(ctx context.Context, lastSeenBefore time.Time, limit int) ([]*UploadSession, error)
	// ListRecoverable returns interrupted sessions eligible for server-side
	// finalization (>= min duration) or cleanup.
	ListRecoverable(ctx context.Context, statuses []SessionStatus, limit int) ([]*UploadSession, error)
	// ListCleanupCandidates returns terminal/interrupted sessions whose temp
	// segments may be reclaimed based on the provided retention cutoffs.
	ListCleanupCandidates(ctx context.Context, q CleanupQuery, limit int) ([]*UploadSession, error)
}

// CleanupQuery describes the retention cutoffs the cleanup task applies. A
// session is a candidate when it matches ANY of the populated rules.
type CleanupQuery struct {
	// AbortedBefore reclaims aborted sessions older than this instant.
	AbortedBefore time.Time
	// CompletedBefore reclaims completed sessions whose temp segments are older
	// than this instant (the formal audio already exists).
	CompletedBefore time.Time
	// FailedBefore reclaims failed sessions older than this instant.
	FailedBefore time.Time
	// InterruptedBefore reclaims interrupted/pending_resume sessions whose
	// last activity is older than this instant (resume retention exceeded).
	InterruptedBefore time.Time
}
