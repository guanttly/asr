package workflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
var maxTokenRangePattern = regexp.MustCompile(`Range of max_tokens should be \[(\d+),\s*(\d+)\]`)
var markdownHeadingPattern = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+\S`)
var markdownListPattern = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+\S|\d+\.\s+\S)`)
var markdownFencePattern = regexp.MustCompile("(?m)^```.*$")

func NewLLMCorrectionHandler() *LLMCorrectionHandler {
	return &LLMCorrectionHandler{
		httpClient: &http.Client{},
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
	cfg, prompt, temperature, maxTokens, endpoint, err := h.prepareRequest(config, inputText)
	if err != nil {
		return inputText, nil, err
	}
	respBody, usedMaxTokens, err := h.executeWithContextRetry(ctx, endpoint, cfg, prompt, temperature, maxTokens)
	if err != nil {
		return inputText, nil, err
	}
	return parseLLMResponse(respBody, cfg.AllowMarkdown, usedMaxTokens)
}

func (h *LLMCorrectionHandler) ExecuteStream(ctx context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta, emit StreamEmitter) (string, json.RawMessage, error) {
	cfg, prompt, temperature, maxTokens, endpoint, err := h.prepareRequest(config, inputText)
	if err != nil {
		return inputText, nil, err
	}
	return h.executeWithContextRetryStream(ctx, endpoint, cfg, prompt, temperature, maxTokens, emit)
}

func (h *LLMCorrectionHandler) prepareRequest(config json.RawMessage, inputText string) (LLMCorrectionConfig, string, float64, int, string, error) {
	var cfg LLMCorrectionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return LLMCorrectionConfig{}, "", 0, 0, "", err
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
		return LLMCorrectionConfig{}, "", 0, 0, "", err
	}
	return cfg, prompt, temperature, maxTokens, endpoint, nil
}

func parseLLMResponse(respBody []byte, allowMarkdown bool, usedMaxTokens int) (string, json.RawMessage, error) {
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
		return "", nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", nil, fmt.Errorf("LLM returned no choices")
	}

	rawContent := chatResp.Choices[0].Message.Content
	var outputText string
	if allowMarkdown {
		outputText = strings.TrimSpace(rawContent)
	} else {
		outputText = normalizeLLMCorrectionOutput(rawContent)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"model":               chatResp.Model,
		"prompt_tokens":       chatResp.Usage.PromptTokens,
		"completion_tokens":   chatResp.Usage.CompletionTokens,
		"max_tokens":          usedMaxTokens,
		"allow_markdown":      allowMarkdown,
		"normalized_markdown": hasMarkdownSyntax(rawContent),
	})

	return outputText, detail, nil
}

func (h *LLMCorrectionHandler) executeWithContextRetryStream(ctx context.Context, endpoint string, cfg LLMCorrectionConfig, prompt string, temperature float64, maxTokens int, emit StreamEmitter) (string, json.RawMessage, error) {
	outputText, detail, respBody, statusCode, err := h.executeChatRequestStream(ctx, endpoint, cfg, prompt, temperature, maxTokens, emit)
	if err != nil {
		return outputText, detail, err
	}
	if statusCode == http.StatusOK {
		return outputText, detail, nil
	}

	allowedTokens, ok := inferRetryMaxTokens(string(respBody), maxTokens)
	if statusCode == http.StatusBadRequest && ok {
		retryOutput, retryDetail, _, _, retryErr := h.executeChatRequestStream(ctx, endpoint, cfg, prompt, temperature, allowedTokens, emit)
		return retryOutput, retryDetail, retryErr
	}

	return outputText, detail, fmt.Errorf("LLM request to %s returned status %d: %s", endpoint, statusCode, string(respBody))
}

func (h *LLMCorrectionHandler) executeWithContextRetry(ctx context.Context, endpoint string, cfg LLMCorrectionConfig, prompt string, temperature float64, maxTokens int) ([]byte, int, error) {
	respBody, statusCode, err := h.executeChatRequest(ctx, endpoint, cfg, prompt, temperature, maxTokens)
	if err != nil {
		return nil, maxTokens, err
	}
	if statusCode == http.StatusOK {
		return respBody, maxTokens, nil
	}

	allowedTokens, ok := inferRetryMaxTokens(string(respBody), maxTokens)
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

func (h *LLMCorrectionHandler) executeChatRequestStream(ctx context.Context, endpoint string, cfg LLMCorrectionConfig, prompt string, temperature float64, maxTokens int, emit StreamEmitter) (string, json.RawMessage, []byte, int, error) {
	reqBody := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": temperature,
		"max_tokens":  maxTokens,
		"stream":      true,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", nil, nil, 0, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", nil, nil, resp.StatusCode, fmt.Errorf("failed to read LLM response: %w", readErr)
		}
		return "", nil, respBody, resp.StatusCode, nil
	}

	if strings.Contains(contentType, "text/event-stream") {
		outputText, detail, streamErr := h.consumeChatStream(resp.Body, cfg.AllowMarkdown, maxTokens, emit)
		return outputText, detail, nil, resp.StatusCode, streamErr
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, nil, resp.StatusCode, fmt.Errorf("failed to read LLM response: %w", err)
	}
	outputText, detail, parseErr := parseLLMResponse(respBody, cfg.AllowMarkdown, maxTokens)
	return outputText, detail, respBody, resp.StatusCode, parseErr
}

func (h *LLMCorrectionHandler) consumeChatStream(body io.Reader, allowMarkdown bool, usedMaxTokens int, emit StreamEmitter) (string, json.RawMessage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var (
		eventLines       []string
		rawContent       strings.Builder
		model            string
		promptTokens     int
		completionTokens int
	)

	flushEvent := func() (bool, error) {
		if len(eventLines) == 0 {
			return false, nil
		}
		payload := strings.Join(eventLines, "\n")
		eventLines = eventLines[:0]
		delta, done, eventModel, promptUsage, completionUsage, err := parseOpenAIStreamEvent(payload)
		if err != nil {
			return false, err
		}
		if eventModel != "" {
			model = eventModel
		}
		if promptUsage > 0 {
			promptTokens = promptUsage
		}
		if completionUsage > 0 {
			completionTokens = completionUsage
		}
		if delta != "" {
			rawContent.WriteString(delta)
			if emit != nil {
				if err := emit(&NodeStreamEvent{
					Type:       NodeStreamEventDelta,
					Delta:      delta,
					OutputText: rawContent.String(),
				}); err != nil {
					return false, err
				}
			}
		}
		return done, nil
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			done, err := flushEvent()
			if err != nil {
				return rawContent.String(), nil, err
			}
			if done {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			eventLines = append(eventLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return rawContent.String(), nil, fmt.Errorf("failed to read LLM stream: %w", err)
	}
	if _, err := flushEvent(); err != nil {
		return rawContent.String(), nil, err
	}

	rawOutput := rawContent.String()
	outputText := strings.TrimSpace(rawOutput)
	if !allowMarkdown {
		outputText = normalizeLLMCorrectionOutput(rawOutput)
	}
	detail, _ := json.Marshal(map[string]interface{}{
		"model":               model,
		"prompt_tokens":       promptTokens,
		"completion_tokens":   completionTokens,
		"max_tokens":          usedMaxTokens,
		"allow_markdown":      allowMarkdown,
		"normalized_markdown": hasMarkdownSyntax(rawOutput),
		"streamed":            true,
	})
	return outputText, detail, nil
}

func parseOpenAIStreamEvent(payload string) (string, bool, string, int, int, error) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return "", false, "", 0, 0, nil
	}
	if trimmed == "[DONE]" {
		return "", true, "", 0, 0, nil
	}

	var chunk struct {
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
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
	if err := json.Unmarshal([]byte(trimmed), &chunk); err != nil {
		return "", false, "", 0, 0, fmt.Errorf("failed to parse LLM stream event: %w", err)
	}

	delta := ""
	if len(chunk.Choices) > 0 {
		delta = stringifyStreamContent(chunk.Choices[0].Delta.Content)
		if delta == "" {
			delta = chunk.Choices[0].Message.Content
		}
	}
	return delta, false, chunk.Model, chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, nil
}

func stringifyStreamContent(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, _ := part["text"].(string); text != "" {
				builder.WriteString(text)
				continue
			}
			if content, _ := part["content"].(string); content != "" {
				builder.WriteString(content)
			}
		}
		return builder.String()
	default:
		return ""
	}
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

func inferRetryMaxTokens(respBody string, requestedMaxTokens int) (int, bool) {
	if allowedTokens, ok := inferAllowedCompletionTokens(respBody, requestedMaxTokens); ok {
		return allowedTokens, true
	}

	rangeMatch := maxTokenRangePattern.FindStringSubmatch(respBody)
	if len(rangeMatch) != 3 {
		return 0, false
	}
	upperBound, err := strconv.Atoi(rangeMatch[2])
	if err != nil || upperBound <= 0 || upperBound >= requestedMaxTokens {
		return 0, false
	}
	return upperBound, true
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
