package meeting

import "time"

// CreateMeetingRequest is the DTO for uploading a new meeting.
type CreateMeetingRequest struct {
	Title    string  `json:"title" binding:"required"`
	AudioURL string  `json:"audio_url" binding:"required"`
	Duration float64 `json:"duration"`
}

// MeetingResponse is the DTO returned to clients.
type MeetingResponse struct {
	ID        uint64    `json:"id"`
	Title     string    `json:"title"`
	Duration  float64   `json:"duration"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// MeetingDetailResponse includes transcripts and summary.
type MeetingDetailResponse struct {
	MeetingResponse
	Transcripts []TranscriptItem `json:"transcripts"`
	Summary     *SummaryItem     `json:"summary,omitempty"`
}

// TranscriptItem is a single transcript segment DTO.
type TranscriptItem struct {
	SpeakerLabel string  `json:"speaker_label"`
	Text         string  `json:"text"`
	StartTime    float64 `json:"start_time"`
	EndTime      float64 `json:"end_time"`
}

// SummaryItem is the meeting summary DTO.
type SummaryItem struct {
	Content      string    `json:"content"`
	ModelVersion string    `json:"model_version"`
	CreatedAt    time.Time `json:"created_at"`
}
