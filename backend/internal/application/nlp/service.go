package nlp

import "context"

// CorrectionEngine defines the port used by the NLP service.
type CorrectionEngine interface {
	Correct(ctx context.Context, dictID *uint64, text string) (string, map[string][]string, error)
}

// SummaryEngine defines the summarizer port.
type SummaryEngine interface {
	Summarize(ctx context.Context, text string) (string, string, error)
}

// Service orchestrates correction and summarization use cases.
type Service struct {
	corrector  CorrectionEngine
	summarizer SummaryEngine
}

// NewService creates a new NLP service.
func NewService(corrector CorrectionEngine, summarizer SummaryEngine) *Service {
	return &Service{corrector: corrector, summarizer: summarizer}
}

// Correct applies the multi-layer terminology correction pipeline.
func (s *Service) Correct(ctx context.Context, req *CorrectRequest) (*CorrectResponse, error) {
	correctedText, corrections, err := s.corrector.Correct(ctx, req.DictID, req.Text)
	if err != nil {
		return nil, err
	}

	return &CorrectResponse{
		OriginalText:  req.Text,
		CorrectedText: correctedText,
		Corrections:   corrections,
	}, nil
}

// Summarize generates a structured summary for transcript text.
func (s *Service) Summarize(ctx context.Context, req *SummarizeRequest) (*SummarizeResponse, error) {
	content, modelVersion, err := s.summarizer.Summarize(ctx, req.Text)
	if err != nil {
		return nil, err
	}

	return &SummarizeResponse{Content: content, ModelVersion: modelVersion}, nil
}
