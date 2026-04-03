package meeting

import "time"

// MeetingStatus represents the processing state of a meeting.
type MeetingStatus string

const (
	MeetingStatusUploaded   MeetingStatus = "uploaded"
	MeetingStatusProcessing MeetingStatus = "processing"
	MeetingStatusCompleted  MeetingStatus = "completed"
	MeetingStatusFailed     MeetingStatus = "failed"
)

// Meeting is the aggregate root for meeting recordings.
type Meeting struct {
	ID           uint64        `json:"id"`
	SourceTaskID *uint64       `json:"source_task_id,omitempty"`
	UserID       uint64        `json:"user_id"`
	Title        string        `json:"title"`
	AudioURL     string        `json:"audio_url"`
	Duration     float64       `json:"duration"` // seconds
	Status       MeetingStatus `json:"status"`
	Transcripts  []Transcript  `json:"transcripts,omitempty"`
	Summary      *Summary      `json:"summary,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// Transcript represents a speaker-attributed segment of the meeting.
type Transcript struct {
	ID           uint64  `json:"id"`
	MeetingID    uint64  `json:"meeting_id"`
	SpeakerLabel string  `json:"speaker_label"`
	Text         string  `json:"text"`
	StartTime    float64 `json:"start_time"` // seconds
	EndTime      float64 `json:"end_time"`
}

// Summary holds the generated meeting summary.
type Summary struct {
	ID           uint64    `json:"id"`
	MeetingID    uint64    `json:"meeting_id"`
	Content      string    `json:"content"`
	ModelVersion string    `json:"model_version"`
	CreatedAt    time.Time `json:"created_at"`
}
