package nlp

// CorrectRequest is the DTO for terminology correction.
type CorrectRequest struct {
	Text   string  `json:"text" binding:"required"`
	DictID *uint64 `json:"dict_id"`
}

// CorrectResponse contains the correction result.
type CorrectResponse struct {
	OriginalText  string              `json:"original_text"`
	CorrectedText string              `json:"corrected_text"`
	Corrections   map[string][]string `json:"corrections"`
}

// SummarizeRequest is the DTO for summary generation.
type SummarizeRequest struct {
	Text string `json:"text" binding:"required"`
}

// SummarizeResponse contains the generated summary.
type SummarizeResponse struct {
	Content      string `json:"content"`
	ModelVersion string `json:"model_version"`
}
