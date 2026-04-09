package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lgt/asr/internal/infrastructure/nlpengine"
)

// TermCorrectionConfig is the configuration for the term correction node.
type TermCorrectionConfig struct {
	DictID uint64 `json:"dict_id"`
}

// TermCorrectionHandler applies terminology-based text correction.
type TermCorrectionHandler struct {
	corrector *nlpengine.Corrector
}

// NewTermCorrectionHandler creates a new term correction handler.
func NewTermCorrectionHandler(corrector *nlpengine.Corrector) *TermCorrectionHandler {
	return &TermCorrectionHandler{corrector: corrector}
}

func (h *TermCorrectionHandler) Validate(config json.RawMessage) error {
	var cfg TermCorrectionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid term_correction config: %w", err)
	}
	if cfg.DictID == 0 {
		return fmt.Errorf("dict_id is required")
	}
	return nil
}

func (h *TermCorrectionHandler) Execute(ctx context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg TermCorrectionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return inputText, nil, err
	}

	corrected, corrections, err := h.corrector.Correct(ctx, &cfg.DictID, inputText)
	if err != nil {
		return inputText, nil, err
	}

	detail, _ := json.Marshal(corrections)
	return corrected, detail, nil
}
