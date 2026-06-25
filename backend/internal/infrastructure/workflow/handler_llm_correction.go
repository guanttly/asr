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
	"unicode/utf8"
)

const (
	// llmCorrectionDefaultMaxTokens is the fallback total token budget when none
	// is configured. The configured value is treated as the model's total token
	// limit (prompt + completion), not just the completion budget.
	llmCorrectionDefaultMaxTokens = 4096
	// llmCorrectionChunkMaxRunes caps the input size per chunk for correction
	// quality, so an over-long context cannot degrade correction or drop
	// paragraphs.
	llmCorrectionChunkMaxRunes = 1600
	// llmCorrectionChunkMinRunes prevents pathological fragmentation when the
	// configured budget is unusually small.
	llmCorrectionChunkMinRunes = 200
	// llmCorrectionTokenSafetyMargin reserves headroom for tokenizer differences
	// between our rune-based estimate and the provider's count.
	llmCorrectionTokenSafetyMargin = 256
	// llmCorrectionMinCompletionTokens is the floor for the derived completion
	// budget.
	llmCorrectionMinCompletionTokens = 256
	// correctionChunkSeparator joins corrected chunks back into one document.
	correctionChunkSeparator = "\n"
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

const defaultLLMPrompt = `你是一个专业的语音转写文本校对助手。请只修正语音识别造成的错别字、同音误识别、标点和明显语序问题，保持原意、语气、人名、数字和专业术语不变。

要求：
1. 只输出纠错后的正文，不要解释、不要标题、不要列表。
2. 如果原文为空或只有空白，直接输出空字符串，不要补充提示语。
3. 无法确定的内容保持原样，不要编造。

原文：
{{TEXT}}`

// LLMCorrectionHandler calls an OpenAI-compatible API for text correction.
type LLMCorrectionHandler struct {
	httpClient *http.Client
}

var maxContextLengthPattern = regexp.MustCompile(`maximum context length is (\d+) tokens`)
var inputTokensPattern = regexp.MustCompile(`request has (\d+) input tokens`)
var maxTokenRangePattern = regexp.MustCompile(`Range of max_tokens should be \[(\d+),\s*(\d+)\]`)
var textPlaceholderPattern = regexp.MustCompile(`(?i)\{\{\s*text\s*\}\}`)
var markdownHeadingPattern = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+\S`)
var markdownListPattern = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+\S|\d+\.\s+\S)`)
var markdownFencePattern = regexp.MustCompile("(?m)^```.*$")
var thinkBlockPattern = regexp.MustCompile(`(?is)<think>.*?</think>`)

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

// correctionChunkDebug mirrors the JSON shape consumed by the frontend
// NodeDetailPanel (chunk_outputs) so chunked correction reuses the per-chunk
// tabbed preview.
type correctionChunkDebug struct {
	Title      string `json:"title"`
	Output     string `json:"output"`
	Prompt     string `json:"prompt"`
	InputRunes int    `json:"input_runes"`
}

// Execute runs the LLM correction node. Long input is split into chunks so the
// model corrects every paragraph (avoiding omissions and quality loss from an
// over-long context), and the per-request completion budget is derived from the
// configured token limit minus the prompt length so callers no longer have to
// subtract the prompt tokens by hand.
func (h *LLMCorrectionHandler) Execute(ctx context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	if strings.TrimSpace(inputText) == "" {
		return "", emptyLLMCorrectionDetail(false), nil
	}

	cfg, endpoint, err := resolveLLMConfig(config)
	if err != nil {
		return inputText, nil, err
	}
	return h.runCorrection(ctx, cfg, endpoint, inputText, false, nil)
}

func (h *LLMCorrectionHandler) ExecuteStream(ctx context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta, emit StreamEmitter) (string, json.RawMessage, error) {
	if strings.TrimSpace(inputText) == "" {
		return "", emptyLLMCorrectionDetail(true), nil
	}

	cfg, endpoint, err := resolveLLMConfig(config)
	if err != nil {
		return inputText, nil, err
	}
	return h.runCorrection(ctx, cfg, endpoint, inputText, true, emit)
}

// ExecuteSingle performs a single correction request without input chunking,
// sending the configured MaxTokens directly as the completion budget. Callers
// that manage their own chunking and token budgets (e.g. the meeting summary
// node) use this to avoid double chunking.
func (h *LLMCorrectionHandler) ExecuteSingle(ctx context.Context, config json.RawMessage, inputText string) (string, json.RawMessage, error) {
	if strings.TrimSpace(inputText) == "" {
		return "", emptyLLMCorrectionDetail(false), nil
	}
	cfg, endpoint, err := resolveLLMConfig(config)
	if err != nil {
		return inputText, nil, err
	}
	prompt := buildCorrectionPrompt(cfg, inputText)
	output, detail, _, err := h.completeOnce(ctx, cfg, endpoint, prompt, cfg.MaxTokens)
	if err != nil {
		return inputText, nil, err
	}
	return output, detail, nil
}

// ExecuteSingleStream is the streaming counterpart of ExecuteSingle.
func (h *LLMCorrectionHandler) ExecuteSingleStream(ctx context.Context, config json.RawMessage, inputText string, emit StreamEmitter) (string, json.RawMessage, error) {
	if strings.TrimSpace(inputText) == "" {
		return "", emptyLLMCorrectionDetail(true), nil
	}
	cfg, endpoint, err := resolveLLMConfig(config)
	if err != nil {
		return inputText, nil, err
	}
	prompt := buildCorrectionPrompt(cfg, inputText)
	return h.completeOnceStream(ctx, cfg, endpoint, prompt, cfg.MaxTokens, emit)
}

// runCorrection decides between a single request and chunked correction, then
// runs each chunk with a prompt-aware completion budget. The streaming flag
// selects the SSE path (used by ExecuteStream) regardless of whether emit is nil.
func (h *LLMCorrectionHandler) runCorrection(ctx context.Context, cfg LLMCorrectionConfig, endpoint, inputText string, streaming bool, emit StreamEmitter) (string, json.RawMessage, error) {
	budget := cfg.MaxTokens
	templateTokens := estimatePromptTokens(buildCorrectionPrompt(cfg, ""))
	chunkRunes := correctionChunkRunes(templateTokens, budget)
	chunks := splitCorrectionChunks(inputText, chunkRunes)

	if len(chunks) <= 1 {
		text := strings.TrimSpace(inputText)
		if len(chunks) == 1 {
			text = chunks[0]
		}
		prompt := buildCorrectionPrompt(cfg, text)
		completion := correctionCompletionTokens(estimatePromptTokens(prompt), budget)
		if streaming {
			return h.completeOnceStream(ctx, cfg, endpoint, prompt, completion, emit)
		}
		output, detail, _, err := h.completeOnce(ctx, cfg, endpoint, prompt, completion)
		if err != nil {
			return inputText, nil, err
		}
		return output, detail, nil
	}

	corrected := make([]string, 0, len(chunks))
	chunkDebug := make([]correctionChunkDebug, 0, len(chunks))
	totalPromptTokens, totalCompletionTokens := 0, 0
	model := ""
	completionCap := budget
	for index, chunk := range chunks {
		if emit != nil {
			if err := emit(&NodeStreamEvent{
				Type:    NodeStreamEventStatus,
				Message: fmt.Sprintf("正在校对第 %d/%d 段...", index+1, len(chunks)),
			}); err != nil {
				return inputText, nil, err
			}
		}
		prompt := buildCorrectionPrompt(cfg, chunk)
		completion := correctionCompletionTokens(estimatePromptTokens(prompt), budget)
		if completion > completionCap {
			completion = completionCap
		}

		var (
			out    string
			detail json.RawMessage
			used   int
			err    error
		)
		if streaming {
			prefix := joinCorrectedChunks(corrected)
			if prefix != "" {
				prefix += correctionChunkSeparator
			}
			out, detail, err = h.completeOnceStream(ctx, cfg, endpoint, prompt, completion, emitWithPrefix(emit, prefix))
		} else {
			out, detail, used, err = h.completeOnce(ctx, cfg, endpoint, prompt, completion)
			if used > 0 && used < completionCap {
				completionCap = used
			}
		}
		if err != nil {
			return inputText, nil, fmt.Errorf("第 %d/%d 段校对失败: %w", index+1, len(chunks), err)
		}

		out = strings.TrimSpace(out)
		corrected = append(corrected, out)
		promptTokens, completionTokens, chunkModel := extractTokenUsage(detail)
		totalPromptTokens += promptTokens
		totalCompletionTokens += completionTokens
		if chunkModel != "" {
			model = chunkModel
		}
		chunkDebug = append(chunkDebug, correctionChunkDebug{
			Title:      fmt.Sprintf("第 %d 段", index+1),
			Output:     out,
			Prompt:     prompt,
			InputRunes: utf8.RuneCountInString(chunk),
		})
	}

	finalText := joinCorrectedChunks(corrected)
	detail, _ := json.Marshal(map[string]interface{}{
		"model":             model,
		"prompt_tokens":     totalPromptTokens,
		"completion_tokens": totalCompletionTokens,
		"max_tokens":        budget,
		"allow_markdown":    cfg.AllowMarkdown,
		"chunked":           true,
		"chunk_count":       len(chunks),
		"chunk_outputs":     chunkDebug,
	})
	return finalText, detail, nil
}

// completeOnce performs a single completion request (with the existing reactive
// context retry) and returns the parsed output plus the max_tokens actually used.
func (h *LLMCorrectionHandler) completeOnce(ctx context.Context, cfg LLMCorrectionConfig, endpoint, prompt string, completionMaxTokens int) (string, json.RawMessage, int, error) {
	respBody, used, err := h.executeWithContextRetry(ctx, endpoint, cfg, prompt, cfg.Temperature, completionMaxTokens)
	if err != nil {
		return "", nil, used, err
	}
	output, detail, parseErr := parseLLMResponse(respBody, cfg.AllowMarkdown, used)
	return output, detail, used, parseErr
}

func (h *LLMCorrectionHandler) completeOnceStream(ctx context.Context, cfg LLMCorrectionConfig, endpoint, prompt string, completionMaxTokens int, emit StreamEmitter) (string, json.RawMessage, error) {
	return h.executeWithContextRetryStream(ctx, endpoint, cfg, prompt, cfg.Temperature, completionMaxTokens, emit)
}

// resolveLLMConfig parses the node config and applies defaults for the prompt
// template, temperature and token budget, and normalizes the endpoint.
func resolveLLMConfig(config json.RawMessage) (LLMCorrectionConfig, string, error) {
	var cfg LLMCorrectionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return LLMCorrectionConfig{}, "", err
	}
	if strings.TrimSpace(cfg.PromptTemplate) == "" {
		cfg.PromptTemplate = defaultLLMPrompt
	}
	if cfg.Temperature <= 0 {
		cfg.Temperature = 0.3
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = llmCorrectionDefaultMaxTokens
	}
	endpoint, err := normalizeOpenAIChatEndpoint(cfg.Endpoint)
	if err != nil {
		return LLMCorrectionConfig{}, "", err
	}
	return cfg, endpoint, nil
}

// buildCorrectionPrompt renders the prompt template with the given text and
// enforces plain-text output when Markdown is not allowed.
func buildCorrectionPrompt(cfg LLMCorrectionConfig, text string) string {
	prompt := renderTextPrompt(cfg.PromptTemplate, text, "原文")
	if !cfg.AllowMarkdown {
		prompt = enforcePlainTextOutput(prompt)
	}
	return prompt
}

// correctionChunkRunes returns the maximum input runes per chunk, reserving room
// for the prompt template, an equally sized corrected output, and a safety
// margin (template + 2*chunk + margin <= budget), capped for correction quality.
func correctionChunkRunes(templateTokens, budget int) int {
	available := budget - templateTokens - llmCorrectionTokenSafetyMargin
	chunkRunes := available / 2
	if chunkRunes > llmCorrectionChunkMaxRunes {
		chunkRunes = llmCorrectionChunkMaxRunes
	}
	if chunkRunes < llmCorrectionChunkMinRunes {
		chunkRunes = llmCorrectionChunkMinRunes
	}
	return chunkRunes
}

// correctionCompletionTokens derives the completion budget from the total token
// limit minus the rendered prompt tokens, so the prompt is always accounted for.
func correctionCompletionTokens(promptTokens, budget int) int {
	completion := budget - promptTokens - llmCorrectionTokenSafetyMargin
	if completion < llmCorrectionMinCompletionTokens {
		completion = llmCorrectionMinCompletionTokens
	}
	return completion
}

// splitCorrectionChunks splits text into chunks of at most maxRunes runes on
// sentence/paragraph boundaries so corrections never cut a sentence in half.
func splitCorrectionChunks(text string, maxRunes int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if maxRunes <= 0 || utf8.RuneCountInString(trimmed) <= maxRunes {
		return []string{trimmed}
	}

	chunks := make([]string, 0)
	var builder strings.Builder
	builderRunes := 0
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		segment := strings.TrimSpace(builder.String())
		if segment != "" {
			chunks = append(chunks, segment)
		}
		builder.Reset()
		builderRunes = 0
	}

	for _, sentence := range splitCorrectionSentences(trimmed) {
		sentenceRunes := utf8.RuneCountInString(sentence)
		if sentenceRunes > maxRunes {
			flush()
			runes := []rune(sentence)
			for start := 0; start < len(runes); start += maxRunes {
				end := start + maxRunes
				if end > len(runes) {
					end = len(runes)
				}
				segment := strings.TrimSpace(string(runes[start:end]))
				if segment != "" {
					chunks = append(chunks, segment)
				}
			}
			continue
		}
		if builderRunes > 0 && builderRunes+sentenceRunes > maxRunes {
			flush()
		}
		builder.WriteString(sentence)
		builderRunes += sentenceRunes
	}
	flush()

	if len(chunks) == 0 {
		return []string{trimmed}
	}
	return chunks
}

// splitCorrectionSentences splits text into sentence-ish segments, keeping the
// trailing delimiter attached so concatenation reconstructs the original text.
func splitCorrectionSentences(text string) []string {
	sentences := make([]string, 0)
	var builder strings.Builder
	for _, r := range text {
		builder.WriteRune(r)
		if isCorrectionSentenceBoundary(r) {
			sentences = append(sentences, builder.String())
			builder.Reset()
		}
	}
	if builder.Len() > 0 {
		sentences = append(sentences, builder.String())
	}
	return sentences
}

func isCorrectionSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '\n', '!', '?', ';':
		return true
	default:
		return false
	}
}

func joinCorrectedChunks(chunks []string) string {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, correctionChunkSeparator)
}

// emitWithPrefix wraps an emitter so streamed delta previews include the text of
// already-corrected chunks.
func emitWithPrefix(emit StreamEmitter, prefix string) StreamEmitter {
	if emit == nil || prefix == "" {
		return emit
	}
	return func(event *NodeStreamEvent) error {
		if event != nil && event.Type == NodeStreamEventDelta {
			clone := *event
			clone.OutputText = prefix + event.OutputText
			return emit(&clone)
		}
		return emit(event)
	}
}

func extractTokenUsage(detail json.RawMessage) (int, int, string) {
	if len(detail) == 0 {
		return 0, 0, ""
	}
	var parsed struct {
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		Model            string `json:"model"`
	}
	if err := json.Unmarshal(detail, &parsed); err != nil {
		return 0, 0, ""
	}
	return parsed.PromptTokens, parsed.CompletionTokens, parsed.Model
}

func renderTextPrompt(template string, inputText string, fallbackLabel string) string {
	if textPlaceholderPattern.MatchString(template) {
		return textPlaceholderPattern.ReplaceAllStringFunc(template, func(string) string {
			return inputText
		})
	}

	trimmedPrompt := strings.TrimSpace(template)
	trimmedInput := strings.TrimSpace(inputText)
	if trimmedInput == "" {
		return trimmedPrompt
	}
	if trimmedPrompt == "" {
		return inputText
	}
	label := strings.TrimSpace(fallbackLabel)
	if label == "" {
		label = "原文"
	}
	return trimmedPrompt + "\n\n" + label + "：\n" + inputText
}

func emptyLLMCorrectionDetail(streamed bool) json.RawMessage {
	detail, _ := json.Marshal(map[string]interface{}{
		"skipped":  true,
		"reason":   "empty_input",
		"streamed": streamed,
	})
	return detail
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
	rawContent = stripReasoningBlocks(rawContent)
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
		emittedClean     string
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
				// Hide qwen3 <think> reasoning from the live preview by emitting
				// the cleaned accumulated text; only surface newly revealed text.
				clean := stripReasoningBlocks(rawContent.String())
				if clean != emittedClean {
					visibleDelta := clean
					if strings.HasPrefix(clean, emittedClean) {
						visibleDelta = clean[len(emittedClean):]
					}
					emittedClean = clean
					if err := emit(&NodeStreamEvent{
						Type:       NodeStreamEventDelta,
						Delta:      visibleDelta,
						OutputText: clean,
					}); err != nil {
						return false, err
					}
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

	rawOutput := stripReasoningBlocks(rawContent.String())
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

// stripReasoningBlocks removes qwen3 / DeepSeek-R1 style <think>...</think>
// reasoning blocks that some OpenAI-compatible servers emit inline in the
// message content (instead of in the separate reasoning_content channel).
// Without stripping, the reasoning text pollutes the corrected output and the
// LLM correction node appears to "fail" (bug 14853). It also tolerates partial
// leaks: a lone trailing </think> (reasoning streamed without an opening tag)
// or a lone <think> with no close (thinking truncated before an answer).
func stripReasoningBlocks(text string) string {
	if text == "" {
		return text
	}
	cleaned := thinkBlockPattern.ReplaceAllString(text, "")

	lower := strings.ToLower(cleaned)
	if idx := strings.LastIndex(lower, "</think>"); idx >= 0 {
		cleaned = cleaned[idx+len("</think>"):]
		lower = strings.ToLower(cleaned)
	}
	if idx := strings.Index(lower, "<think>"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	return cleaned
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
