package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sensitivedomain "github.com/lgt/asr/internal/domain/sensitive"
	"gorm.io/gorm"
)

type sensitiveWordMeta struct {
	Sources     []string `json:"sources"`
	SourceTypes []string `json:"source_types"`
}

type sensitiveMatchedWord struct {
	Word        string   `json:"word"`
	Matches     int      `json:"matches"`
	Sources     []string `json:"sources,omitempty"`
	SourceTypes []string `json:"source_types,omitempty"`
}

// SensitiveFilterConfig is the configuration for the sensitive word filter node.
type SensitiveFilterConfig struct {
	DictID      uint64   `json:"dict_id"`
	Words       []string `json:"words"`
	CustomWords []string `json:"custom_words,omitempty"`
	Replacement string   `json:"replacement"`
}

// DefaultSensitiveReplacement is used when no replacement is configured.
const DefaultSensitiveReplacement = "[已过滤]"

// SensitiveFilterHandler masks configured sensitive words in text.
type SensitiveFilterHandler struct {
	dictRepo  sensitivedomain.DictRepository
	entryRepo sensitivedomain.EntryRepository
}

func NewSensitiveFilterHandler(dictRepo sensitivedomain.DictRepository, entryRepo sensitivedomain.EntryRepository) *SensitiveFilterHandler {
	return &SensitiveFilterHandler{dictRepo: dictRepo, entryRepo: entryRepo}
}

func (h *SensitiveFilterHandler) Validate(config json.RawMessage) error {
	if len(config) == 0 || string(config) == "null" {
		return nil
	}
	var cfg SensitiveFilterConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid sensitive_filter config: %w", err)
	}
	return nil
}

func (h *SensitiveFilterHandler) Execute(ctx context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg SensitiveFilterConfig
	if len(config) > 0 && string(config) != "null" {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return inputText, nil, fmt.Errorf("invalid sensitive_filter config: %w", err)
		}
	}

	replacement := strings.TrimSpace(cfg.Replacement)
	if replacement == "" {
		replacement = DefaultSensitiveReplacement
	}

	wordSet := make(map[string]struct{})
	wordMeta := make(map[string]*sensitiveWordMeta)
	if cfg.DictID > 0 && h.dictRepo != nil {
		if _, err := h.dictRepo.GetByID(ctx, cfg.DictID); err != nil {
			if err == gorm.ErrRecordNotFound {
				return inputText, nil, fmt.Errorf("sensitive dict %d not found", cfg.DictID)
			}
			return inputText, nil, err
		}
	}
	if h.entryRepo != nil {
		entries, err := h.entryRepo.ListAppliedByDict(ctx, cfg.DictID)
		if err != nil {
			return inputText, nil, err
		}
		dictCache := make(map[uint64]*sensitivedomain.Dict)
		for _, entry := range entries {
			if word := strings.TrimSpace(entry.Word); word != "" {
				wordSet[word] = struct{}{}
				dictItem, ok := dictCache[entry.DictID]
				if !ok && h.dictRepo != nil {
					dictItem, err = h.dictRepo.GetByID(ctx, entry.DictID)
					if err != nil {
						return inputText, nil, err
					}
					dictCache[entry.DictID] = dictItem
				}
				sourceLabel := "敏感词库"
				sourceType := "dict"
				if dictItem != nil {
					sourceLabel = dictItem.Name
					if dictItem.IsBase {
						sourceType = "base_dict"
					} else {
						sourceType = "scene_dict"
					}
				}
				appendSensitiveWordMeta(wordMeta, word, sourceLabel, sourceType)
			}
		}
	}
	for _, word := range cfg.Words {
		if word = strings.TrimSpace(word); word != "" {
			wordSet[word] = struct{}{}
			appendSensitiveWordMeta(wordMeta, word, "兼容内联词", "inline_words")
		}
	}
	for _, word := range cfg.CustomWords {
		if word = strings.TrimSpace(word); word != "" {
			wordSet[word] = struct{}{}
			appendSensitiveWordMeta(wordMeta, word, "节点自定义补充词", "custom_words")
		}
	}

	result := inputText
	masked := make([]string, 0, len(wordSet))
	matchedWords := make([]sensitiveMatchedWord, 0, len(wordSet))
	sortedWords := make([]string, 0, len(wordSet))
	for word := range wordSet {
		sortedWords = append(sortedWords, word)
	}
	for i := 0; i < len(sortedWords); i++ {
		for j := i + 1; j < len(sortedWords); j++ {
			if len(sortedWords[i]) < len(sortedWords[j]) {
				sortedWords[i], sortedWords[j] = sortedWords[j], sortedWords[i]
			}
		}
	}

	for _, word := range sortedWords {
		if strings.Contains(result, word) {
			count := strings.Count(result, word)
			result = strings.ReplaceAll(result, word, replacement)
			masked = append(masked, fmt.Sprintf("%s (x%d)", word, count))
			matched := sensitiveMatchedWord{Word: word, Matches: count}
			if meta := wordMeta[word]; meta != nil {
				matched.Sources = append(matched.Sources, meta.Sources...)
				matched.SourceTypes = append(matched.SourceTypes, meta.SourceTypes...)
			}
			matchedWords = append(matchedWords, matched)
		}
	}

	detail, _ := json.Marshal(map[string]any{
		"masked_words":  masked,
		"matched_words": matchedWords,
		"replacement":   replacement,
		"words_used":    len(sortedWords),
	})
	return cleanupSpaces(result), detail, nil
}

func appendSensitiveWordMeta(wordMeta map[string]*sensitiveWordMeta, word string, source string, sourceType string) {
	item := wordMeta[word]
	if item == nil {
		item = &sensitiveWordMeta{}
		wordMeta[word] = item
	}
	if !containsString(item.Sources, source) {
		item.Sources = append(item.Sources, source)
	}
	if !containsString(item.SourceTypes, sourceType) {
		item.SourceTypes = append(item.SourceTypes, sourceType)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
