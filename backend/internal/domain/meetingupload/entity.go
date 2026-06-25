// Package meetingupload models resumable, crash-safe meeting upload sessions.
//
// A long meeting recording is streamed to the server progressively as a series
// of raw-PCM segments. Each segment is persisted to disk and tracked in the
// database so that an in-progress upload survives a backend restart and can be
// resumed by the client (or recovered server-side) after a crash or network
// drop. This is what guarantees a recording is never lost once it reaches the
// server.
package meetingupload

import "time"

// SessionStatus is the lifecycle state of an upload session.
type SessionStatus string

const (
	// SessionStatusRecording means the client is actively uploading chunks (or
	// is briefly idle but still within the heartbeat window). Never cleaned up.
	SessionStatusRecording SessionStatus = "recording"
	// SessionStatusInterrupted means the client lost its heartbeat (crash /
	// network drop). Segments are preserved and the session may be resumed or
	// recovered server-side. Cleaned up only after the resume retention window.
	SessionStatusInterrupted SessionStatus = "interrupted"
	// SessionStatusCompleting means complete() was requested and the audio is
	// being assembled.
	SessionStatusCompleting SessionStatus = "completing"
	// SessionStatusCompleted means the audio was assembled and a meeting was
	// created / finalized. Temp segments are removed after the retention window.
	SessionStatusCompleted SessionStatus = "completed"
	// SessionStatusAborted means the client discarded the recording (or it was
	// shorter than the minimum duration when stopped). Temp segments are removed
	// immediately.
	SessionStatusAborted SessionStatus = "aborted"
	// SessionStatusExpired means the session was reclaimed by the cleanup task
	// after exceeding its retention window without recovery.
	SessionStatusExpired SessionStatus = "expired"
	// SessionStatusFailed means assembling or finalizing the audio failed.
	SessionStatusFailed SessionStatus = "failed"
)

// SegmentStatus is the state of a single uploaded segment.
type SegmentStatus string

const (
	// SegmentStatusStored means the segment file is on disk and accounted for.
	SegmentStatusStored SegmentStatus = "stored"
	// SegmentStatusMerged means the segment was folded into the final audio and
	// its temp file may be removed.
	SegmentStatusMerged SegmentStatus = "merged"
)

// FormatPCMS16LE16kMono is the canonical raw-PCM format the desktop client
// streams: 16-bit signed little-endian, 16 kHz, mono. The server assembles the
// segments and prepends a WAV header at completion.
const FormatPCMS16LE16kMono = "pcm_s16le_16000_mono"

// UploadSession tracks the durable state of one in-progress meeting upload.
type UploadSession struct {
	ID         uint64
	UploadID   string // opaque client-facing token
	UserID     uint64
	MeetingID  *uint64 // set once promoted to a real meeting (>= min duration)
	Status     SessionStatus
	Format     string
	Filename   string
	Title      string
	WorkflowID *uint64
	Language   string

	DurationSec float64
	TotalBytes  int64
	NextIndex   int

	// PublicBaseURL is captured from the init request so background tasks
	// (recovery / cleanup) can build absolute audio URLs without a request.
	PublicBaseURL string

	StartedAt   time.Time
	LastSeenAt  time.Time
	StoppedAt   *time.Time
	CompletedAt *time.Time
	AbortedAt   *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// UploadSegment is one persisted chunk of a session's audio.
type UploadSegment struct {
	ID              uint64
	UploadSessionID uint64
	SegmentIndex    int
	Path            string
	Bytes           int64
	DurationSec     float64
	Checksum        string
	Status          SegmentStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IsActive reports whether the session is still actively recording/uploading.
func (s *UploadSession) IsActive() bool {
	return s.Status == SessionStatusRecording || s.Status == SessionStatusCompleting
}

// IsTerminal reports whether the session has reached a final state.
func (s *UploadSession) IsTerminal() bool {
	switch s.Status {
	case SessionStatusCompleted, SessionStatusAborted, SessionStatusExpired, SessionStatusFailed:
		return true
	default:
		return false
	}
}
