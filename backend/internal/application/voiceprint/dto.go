package voiceprint

// EnrollRequest is the DTO for registering a speaker voiceprint.
type EnrollRequest struct {
	SpeakerName   string
	Department    string
	Notes         string
	AudioFilePath string
}

// Record is the sanitized voiceprint payload exposed to handlers.
type Record struct {
	ID            string  `json:"id"`
	SpeakerName   string  `json:"speaker_name"`
	Department    string  `json:"department"`
	Notes         string  `json:"notes"`
	AudioDuration float64 `json:"audio_duration"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}
