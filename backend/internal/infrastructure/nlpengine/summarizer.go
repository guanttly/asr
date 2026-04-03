package nlpengine

import (
	"context"
	"strings"
	"unicode/utf8"
)

const summaryPreviewMaxRunes = 240

// Summarizer generates lightweight structured summaries.
type Summarizer struct {
	modelVersion string
}

// NewSummarizer creates a placeholder summarizer.
func NewSummarizer(modelVersion string) *Summarizer {
	return &Summarizer{modelVersion: modelVersion}
}

// Summarize returns a deterministic summary stub for early integration.
func (s *Summarizer) Summarize(_ context.Context, text string) (string, string, error) {
	trimmed := strings.ToValidUTF8(strings.TrimSpace(text), "")
	if utf8.RuneCountInString(trimmed) > summaryPreviewMaxRunes {
		runes := []rune(trimmed)
		trimmed = string(runes[:summaryPreviewMaxRunes]) + "..."
	}

	content := "核心内容\n" + trimmed + "\n\n待办事项\n- 待接入真实摘要模型\n\n决议\n- 待确认"
	return content, s.modelVersion, nil
}
