package asr

// AudioChunk represents a single audio segment sent during streaming.
type AudioChunk struct {
	Data       []byte  `json:"data"`
	SampleRate int     `json:"sample_rate"`
	Timestamp  float64 `json:"timestamp"` // seconds from stream start
}

// Timestamp marks a position in the audio.
type Timestamp struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// SpeakerSegment associates a time range with a speaker label.
type SpeakerSegment struct {
	Speaker   string    `json:"speaker"`
	Text      string    `json:"text"`
	Timestamp Timestamp `json:"timestamp"`
}

// StreamingResult is a single incremental result from the ASR engine.
type StreamingResult struct {
	Text      string  `json:"text"`
	IsFinal   bool    `json:"is_final"`
	Timestamp float64 `json:"timestamp"`
}
