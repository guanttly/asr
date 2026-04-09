package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// FillerFilterConfig is the configuration for the filler word filter node.
type FillerFilterConfig struct {
	FilterWords []string `json:"filter_words"`
	CustomWords []string `json:"custom_words,omitempty"`
}

// DefaultFillerWords is the built-in list of Chinese filler/hesitation words.
var DefaultFillerWords = []string{
	"嗯", "啊", "呃", "哦", "哈", "呢", "吧", "嘛",
	"那个", "这个", "就是说", "就是", "然后", "所以说",
	"对对对", "是的是的", "怎么说呢", "我觉得吧",
}

// FillerFilterHandler removes filler/hesitation words from text.
type FillerFilterHandler struct{}

func NewFillerFilterHandler() *FillerFilterHandler {
	return &FillerFilterHandler{}
}

func (h *FillerFilterHandler) Validate(config json.RawMessage) error {
	if len(config) == 0 || string(config) == "null" {
		return nil // Will use defaults
	}
	var cfg FillerFilterConfig
	return json.Unmarshal(config, &cfg)
}

func (h *FillerFilterHandler) Execute(_ context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg FillerFilterConfig
	if len(config) > 0 && string(config) != "null" {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return inputText, nil, fmt.Errorf("invalid filler_filter config: %w", err)
		}
	}

	// Merge word lists
	wordSet := make(map[string]struct{})
	words := cfg.FilterWords
	if len(words) == 0 {
		words = DefaultFillerWords
	}
	for _, w := range words {
		if w = strings.TrimSpace(w); w != "" {
			wordSet[w] = struct{}{}
		}
	}
	for _, w := range cfg.CustomWords {
		if w = strings.TrimSpace(w); w != "" {
			wordSet[w] = struct{}{}
		}
	}

	result := inputText
	var removed []string
	// Remove longer words first to avoid partial matches
	sortedWords := make([]string, 0, len(wordSet))
	for w := range wordSet {
		sortedWords = append(sortedWords, w)
	}
	// Sort by length descending
	for i := 0; i < len(sortedWords); i++ {
		for j := i + 1; j < len(sortedWords); j++ {
			if len(sortedWords[i]) < len(sortedWords[j]) {
				sortedWords[i], sortedWords[j] = sortedWords[j], sortedWords[i]
			}
		}
	}

	for _, w := range sortedWords {
		if strings.Contains(result, w) {
			count := strings.Count(result, w)
			result = strings.ReplaceAll(result, w, "")
			removed = append(removed, fmt.Sprintf("%s (x%d)", w, count))
		}
	}

	// Clean up multiple spaces/punctuation
	result = cleanupSpaces(result)

	detail, _ := json.Marshal(map[string]interface{}{
		"removed_words": removed,
		"words_used":    len(sortedWords),
	})
	return result, detail, nil
}

func cleanupSpaces(s string) string {
	// Collapse multiple spaces into one
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	// Collapse multiple commas
	for strings.Contains(s, "，，") {
		s = strings.ReplaceAll(s, "，，", "，")
	}
	for strings.Contains(s, ",,") {
		s = strings.ReplaceAll(s, ",,", ",")
	}
	return strings.TrimSpace(s)
}
