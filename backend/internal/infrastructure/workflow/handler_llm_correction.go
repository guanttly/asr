package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LLMCorrectionConfig is the configuration for the LLM correction node.
type LLMCorrectionConfig struct {
	Endpoint       string  `json:"endpoint"`
	Model          string  `json:"model"`
	APIKey         string  `json:"api_key,omitempty"`
	PromptTemplate string  `json:"prompt_template"`
	Temperature    float64 `json:"temperature"`
	MaxTokens      int     `json:"max_tokens"`
	AllowMarkdown  bool    `json:"allow_markdown,omitempty"`
}

const defaultLLMPrompt = `你是一个专业的文本校对助手。请对以下语音转写文本进行纠错，修正错别字、语法错误和不通顺的表述，但保持原意不变。只输出纠错后的文本，不要添加任何解释。

原文：
{{TEXT}}`

// LLMCorrectionHandler calls an OpenAI-compatible API for text correction.
type LLMCorrectionHandler struct {
	httpClient *http.Client
}

var maxContextLengthPattern = regexp.MustCompile(`maximum context length is (\d+) tokens`)
var inputTokensPattern = regexp.MustCompile(`request has (\d+) input tokens`)
var markdownHeadingPattern = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+\S`)
var markdownListPattern = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+\S|\d+\.\s+\S)`)
var markdownFencePattern = regexp.MustCompile("(?m)^```.*$")

func NewLLMCorrectionHandler() *LLMCorrectionHandler {
	return &LLMCorrectionHandler{
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

func (h *LLMCorrectionHandler) Validate(config json.RawMessage) error {
	var cfg LLMCorrectionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid llm_correction config: %w", err)
	}
	if cfg.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if err := validateOpenAIChatEndpoint(cfg.Endpoint); err != nil {
		return err
	}
	if cfg.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

func (h *LLMCorrectionHandler) Execute(ctx context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg LLMCorrectionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return inputText, nil, err
	}

	prompt := cfg.PromptTemplate
	if prompt == "" {
		prompt = defaultLLMPrompt
	}
	prompt = strings.ReplaceAll(prompt, "{{TEXT}}", inputText)
	if !cfg.AllowMarkdown {
		prompt = enforcePlainTextOutput(prompt)
	}

	temperature := cfg.Temperature
	if temperature <= 0 {
		temperature = 0.3
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	endpoint, err := normalizeOpenAIChatEndpoint(cfg.Endpoint)
	if err != nil {
		return inputText, nil, err
	}
	respBody, usedMaxTokens, err := h.executeWithContextRetry(ctx, endpoint, cfg, prompt, temperature, maxTokens)
	if err != nil {
		return inputText, nil, err
	}

	// Parse OpenAI response
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return inputText, nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return inputText, nil, fmt.Errorf("LLM returned no choices")
	}

	rawContent := chatResp.Choices[0].Message.Content
	var outputText string
	if cfg.AllowMarkdown {
		outputText = strings.TrimSpace(rawContent)
	} else {
		outputText = normalizeLLMCorrectionOutput(rawContent)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"model":               chatResp.Model,
		"prompt_tokens":       chatResp.Usage.PromptTokens,
		"completion_tokens":   chatResp.Usage.CompletionTokens,
		"max_tokens":          usedMaxTokens,
		"allow_markdown":      cfg.AllowMarkdown,
		"normalized_markdown": hasMarkdownSyntax(rawContent),
	})

	return outputText, detail, nil
}

func (h *LLMCorrectionHandler) executeWithContextRetry(ctx context.Context, endpoint string, cfg LLMCorrectionConfig, prompt string, temperature float64, maxTokens int) ([]byte, int, error) {
	respBody, statusCode, err := h.executeChatRequest(ctx, endpoint, cfg, prompt, temperature, maxTokens)
	if err != nil {
		return nil, maxTokens, err
	}
	if statusCode == http.StatusOK {
		return respBody, maxTokens, nil
	}

	allowedTokens, ok := inferAllowedCompletionTokens(string(respBody), maxTokens)
	if statusCode == http.StatusBadRequest && ok {
		respBody, retryStatusCode, retryErr := h.executeChatRequest(ctx, endpoint, cfg, prompt, temperature, allowedTokens)
		if retryErr != nil {
			return nil, allowedTokens, retryErr
		}
		if retryStatusCode == http.StatusOK {
			return respBody, allowedTokens, nil
		}
		return nil, allowedTokens, fmt.Errorf("LLM request to %s returned status %d after retry with max_tokens=%d: %s", endpoint, retryStatusCode, allowedTokens, string(respBody))
	}

	return nil, maxTokens, fmt.Errorf("LLM request to %s returned status %d: %s", endpoint, statusCode, string(respBody))
}

func (h *LLMCorrectionHandler) executeChatRequest(ctx context.Context, endpoint string, cfg LLMCorrectionConfig, prompt string, temperature float64, maxTokens int) ([]byte, int, error) {
	reqBody := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read LLM response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func inferAllowedCompletionTokens(respBody string, requestedMaxTokens int) (int, bool) {
	maxContextMatch := maxContextLengthPattern.FindStringSubmatch(respBody)
	inputTokensMatch := inputTokensPattern.FindStringSubmatch(respBody)
	if len(maxContextMatch) != 2 || len(inputTokensMatch) != 2 {
		return 0, false
	}

	maxContextTokens, err := strconv.Atoi(maxContextMatch[1])
	if err != nil {
		return 0, false
	}
	inputTokens, err := strconv.Atoi(inputTokensMatch[1])
	if err != nil {
		return 0, false
	}

	allowedTokens := maxContextTokens - inputTokens
	if allowedTokens <= 0 || allowedTokens >= requestedMaxTokens {
		return 0, false
	}
	return allowedTokens, true
}

func validateOpenAIChatEndpoint(raw string) error {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return fmt.Errorf("endpoint is required")
	}
	return nil
}

func normalizeOpenAIChatEndpoint(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if err := validateOpenAIChatEndpoint(trimmed); err != nil {
		return "", err
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, "/chat/completions") {
		return trimmed, nil
	}
	if strings.HasSuffix(lower, "/v1") {
		return trimmed + "/chat/completions", nil
	}
	return trimmed + "/v1/chat/completions", nil
}

func enforcePlainTextOutput(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return trimmed
	}

	guard := "\n\n输出约束：仅返回纠错后的正文，不要解释，不要标题，不要列表，不要 Markdown 语法（不要使用 #、-、*、```）。"
	if strings.Contains(trimmed, "输出约束") || strings.Contains(trimmed, "不要 Markdown") {
		return trimmed
	}
	return trimmed + guard
}

func hasMarkdownSyntax(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "```") {
		return true
	}
	if markdownHeadingPattern.MatchString(trimmed) {
		return true
	}
	return markdownListPattern.MatchString(trimmed)
}

func normalizeLLMCorrectionOutput(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if !hasMarkdownSyntax(trimmed) {
		return trimmed
	}

	cleaned := strings.ReplaceAll(trimmed, "\r\n", "\n")
	cleaned = markdownFencePattern.ReplaceAllString(cleaned, "")

	lines := strings.Split(cleaned, "\n")
	for i := range lines {
		line := strings.TrimSpace(lines[i])
		line = strings.TrimPrefix(line, ">")
		line = strings.TrimSpace(line)

		for strings.HasPrefix(line, "#") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}

		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
			line = strings.TrimSpace(line[2:])
		}

		if dot := strings.Index(line, ". "); dot > 0 {
			allDigits := true
			for _, r := range line[:dot] {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				line = strings.TrimSpace(line[dot+2:])
			}
		}

		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "__", "")
		lines[i] = line
	}

	result := strings.TrimSpace(strings.Join(lines, "\n"))
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}
