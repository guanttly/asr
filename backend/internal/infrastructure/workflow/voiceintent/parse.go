package voiceintent

import (
	"encoding/json"
	"strings"
)

type Result struct {
	WakeMatched bool    `json:"wake_matched,omitempty"`
	WakeWord    string  `json:"wake_word,omitempty"`
	WakeAlias   string  `json:"wake_alias,omitempty"`
	Matched     bool    `json:"matched"`
	Intent      string  `json:"intent"`
	GroupKey    string  `json:"group_key"`
	CommandID   uint64  `json:"command_id"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason"`
	RawOutput   string  `json:"raw_output,omitempty"`
}

func Parse(raw string) (Result, bool) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return Result{}, false
	}
	chunk := trimmed[start : end+1]
	var parsed Result
	if err := json.Unmarshal([]byte(chunk), &parsed); err != nil {
		return Result{}, false
	}
	parsed.Intent = strings.TrimSpace(parsed.Intent)
	parsed.GroupKey = strings.TrimSpace(parsed.GroupKey)
	parsed.WakeWord = strings.TrimSpace(parsed.WakeWord)
	parsed.WakeAlias = strings.TrimSpace(parsed.WakeAlias)
	parsed.Reason = strings.TrimSpace(parsed.Reason)
	parsed.RawOutput = strings.TrimSpace(raw)
	if !parsed.Matched {
		parsed.Intent = ""
		parsed.GroupKey = ""
		parsed.CommandID = 0
	}
	return parsed, true
}
