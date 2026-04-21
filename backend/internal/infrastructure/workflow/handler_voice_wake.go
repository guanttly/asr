package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const VoiceCommandBypassPrefix = "__voice_command__:"

type VoiceWakeConfig struct {
	WakeWords      []string `json:"wake_words"`
	HomophoneWords []string `json:"homophone_words"`
}

type VoiceWakeResult struct {
	WakeMatched bool   `json:"wake_matched"`
	WakeWord    string `json:"wake_word,omitempty"`
	WakeAlias   string `json:"wake_alias,omitempty"`
	Residue     string `json:"residue,omitempty"`
	Bypassed    bool   `json:"bypassed,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type VoiceWakeHandler struct{}

func NewVoiceWakeHandler() *VoiceWakeHandler {
	return &VoiceWakeHandler{}
}

func (h *VoiceWakeHandler) Validate(config json.RawMessage) error {
	var cfg VoiceWakeConfig
	if len(config) > 0 && string(config) != "null" {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid voice_wake config: %w", err)
		}
	}
	if len(buildWakeCandidates(cfg)) == 0 {
		return fmt.Errorf("wake_words or homophone_words is required")
	}
	return nil
}

func (h *VoiceWakeHandler) Execute(_ context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg VoiceWakeConfig
	if len(config) > 0 && string(config) != "null" {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return inputText, nil, fmt.Errorf("invalid voice_wake config: %w", err)
		}
	}
	trimmedInput := strings.TrimSpace(inputText)
	if strings.HasPrefix(trimmedInput, VoiceCommandBypassPrefix) {
		residue := strings.TrimSpace(strings.TrimPrefix(trimmedInput, VoiceCommandBypassPrefix))
		result := VoiceWakeResult{
			WakeMatched: true,
			Bypassed:    true,
			Residue:     residue,
			Reason:      "桌面端已进入指令模式，跳过唤醒词识别",
		}
		return marshalVoiceWakeResult(result)
	}

	candidates := buildWakeCandidates(cfg)
	matchedWord, matchedAlias, residue, matched := matchWakeCandidate(trimmedInput, candidates)
	result := VoiceWakeResult{
		WakeMatched: matched,
		WakeWord:    matchedWord,
		WakeAlias:   matchedAlias,
		Residue:     residue,
	}
	if matched {
		if residue == "" {
			result.Reason = "已命中唤醒词，等待后续指令"
		} else {
			result.Reason = "已命中唤醒词，并提取尾随指令"
		}
	} else {
		result.Reason = "未命中唤醒词"
	}
	return marshalVoiceWakeResult(result)
}

func marshalVoiceWakeResult(result VoiceWakeResult) (string, json.RawMessage, error) {
	output, err := json.Marshal(result)
	if err != nil {
		return "", nil, err
	}
	detail, _ := json.Marshal(map[string]any{
		"wake_matched": result.WakeMatched,
		"wake_word":    result.WakeWord,
		"wake_alias":   result.WakeAlias,
		"residue":      result.Residue,
		"bypassed":     result.Bypassed,
		"reason":       result.Reason,
	})
	return string(output), detail, nil
}

type wakeCandidate struct {
	WakeWord   string
	Alias      string
	Normalized string
}

func buildWakeCandidates(cfg VoiceWakeConfig) []wakeCandidate {
	items := make([]wakeCandidate, 0, len(cfg.WakeWords)+len(cfg.HomophoneWords))
	seen := map[string]struct{}{}
	appendCandidate := func(value string, wakeWord string) {
		trimmed := strings.TrimSpace(value)
		normalized := normalizeWakeText(trimmed)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		items = append(items, wakeCandidate{WakeWord: wakeWord, Alias: trimmed, Normalized: normalized})
	}
	for _, item := range cfg.WakeWords {
		trimmed := strings.TrimSpace(item)
		appendCandidate(trimmed, trimmed)
	}
	defaultWakeWord := ""
	if len(cfg.WakeWords) > 0 {
		defaultWakeWord = strings.TrimSpace(cfg.WakeWords[0])
	}
	for _, item := range cfg.HomophoneWords {
		appendCandidate(item, defaultWakeWord)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if len(items[i].Normalized) == len(items[j].Normalized) {
			return items[i].Alias < items[j].Alias
		}
		return len(items[i].Normalized) > len(items[j].Normalized)
	})
	return items
}

func normalizeWakeText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "，", "", ",", "", "。", "", ".", "", "！", "", "!", "", "？", "", "?", "", "、", "", "-", "", "_", "", "（", "", "）", "", "(", "", ")", "")
	return replacer.Replace(value)
}

func matchWakeCandidate(text string, candidates []wakeCandidate) (wakeWord string, wakeAlias string, residue string, matched bool) {
	normalizedText := normalizeWakeText(text)
	if normalizedText == "" || len(candidates) == 0 {
		return "", "", strings.TrimSpace(text), false
	}
	for _, candidate := range candidates {
		idx := strings.Index(normalizedText, candidate.Normalized)
		if idx < 0 {
			continue
		}
		cutAt := sliceOriginalAfterNormalized(text, idx+len(candidate.Normalized))
		matchedText := strings.TrimSpace(candidate.Alias)
		if cutAt >= 0 {
			return candidate.WakeWord, matchedText, strings.TrimSpace(text[cutAt:]), true
		}
		return candidate.WakeWord, matchedText, "", true
	}
	return "", "", strings.TrimSpace(text), false
}

func sliceOriginalAfterNormalized(text string, target int) int {
	runes := []rune(text)
	consumed := 0
	for index, item := range runes {
		consumed += len([]rune(normalizeWakeText(string(item))))
		if consumed >= target {
			return len(string(runes[:index+1]))
		}
	}
	return -1
}

func parseVoiceWakeResult(raw string) (VoiceWakeResult, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.Contains(trimmed, "wake_matched") {
		return VoiceWakeResult{}, false
	}
	var result VoiceWakeResult
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return VoiceWakeResult{}, false
	}
	result.WakeWord = strings.TrimSpace(result.WakeWord)
	result.WakeAlias = strings.TrimSpace(result.WakeAlias)
	result.Residue = strings.TrimSpace(result.Residue)
	result.Reason = strings.TrimSpace(result.Reason)
	return result, true
}
