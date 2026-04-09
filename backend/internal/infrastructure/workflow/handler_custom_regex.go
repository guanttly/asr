package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
)

// CustomRegexConfig is the configuration for the custom regex replacement node.
type CustomRegexConfig struct {
	Rules []RegexRule `json:"rules"`
}

// RegexRule is a single regex replacement rule.
type RegexRule struct {
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Enabled     bool   `json:"enabled"`
}

// CustomRegexHandler applies user-defined regex replacements to text.
type CustomRegexHandler struct{}

func NewCustomRegexHandler() *CustomRegexHandler {
	return &CustomRegexHandler{}
}

func (h *CustomRegexHandler) Validate(config json.RawMessage) error {
	var cfg CustomRegexConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid custom_regex config: %w", err)
	}
	for i, rule := range cfg.Rules {
		if rule.Pattern == "" {
			continue
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("rule[%d] has invalid regex pattern %q: %w", i, rule.Pattern, err)
		}
	}
	return nil
}

func (h *CustomRegexHandler) Execute(_ context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg CustomRegexConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return inputText, nil, err
	}

	result := inputText
	var applied []map[string]string

	for _, rule := range cfg.Rules {
		if !rule.Enabled || rule.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue // Skip invalid patterns
		}
		matches := re.FindAllString(result, -1)
		if len(matches) > 0 {
			result = re.ReplaceAllString(result, rule.Replacement)
			applied = append(applied, map[string]string{
				"pattern":     rule.Pattern,
				"replacement": rule.Replacement,
				"matches":     fmt.Sprintf("%d", len(matches)),
			})
		}
	}

	detail, _ := json.Marshal(map[string]interface{}{
		"applied_rules": applied,
	})
	return result, detail, nil
}
