package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lgt/asr/internal/infrastructure/nlpengine"
)

// MeetingSummaryConfig is the configuration for the meeting summary node.
type MeetingSummaryConfig struct {
	Endpoint       string `json:"endpoint,omitempty"`
	Model          string `json:"model,omitempty"`
	APIKey         string `json:"api_key,omitempty"`
	PromptTemplate string `json:"prompt_template,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	MaxTokens      int    `json:"max_tokens,omitempty"`
}

// MeetingSummaryHandler generates a meeting summary from text.
// It uses the existing Summarizer as a fallback when no LLM endpoint is configured.
type MeetingSummaryHandler struct {
	summarizer *nlpengine.Summarizer
	llmHandler *LLMCorrectionHandler
}

const (
	meetingSummaryChunkMaxRunes    = 2400
	meetingSummaryChunkMaxTokens   = 1024
	meetingSummaryDefaultMaxTokens = 200000
	meetingSummaryBuiltinSource    = "builtin_summarizer"
	meetingSummaryChunkedLLMSource = "chunked_llm_summary"
	meetingSummaryDirectLLMSource  = "llm_summary"
)

func NewMeetingSummaryHandler(summarizer *nlpengine.Summarizer) *MeetingSummaryHandler {
	return &MeetingSummaryHandler{
		summarizer: summarizer,
		llmHandler: NewLLMCorrectionHandler(),
	}
}

const defaultSummaryPrompt = `# 角色
你是一位资深的会议纪要撰写专家。你的任务是将语音转写的原始文本整理为清晰、专业、可直接用于存档和分发的结构化会议纪要。

# 输出格式要求
- 使用 Markdown 格式输出
- 严格按照下方模板的标题层级和顺序组织内容
- 每个板块如果没有相关内容，省略板块
- 要点使用无序列表（- ），待办和决议使用有序列表（1. ）
- 语言简洁精炼，去除口语化表述和重复内容
- 不要输出任何解释、前言或结尾寒暄

# 输出模板

## 📋 会议概要
> 用 2-3 句话概括本次会议的主题、目的和整体结论。

## 📌 讨论要点
- **议题 1**：核心结论或讨论结果
- **议题 2**：核心结论或讨论结果
- ...（按讨论顺序列出）

## ✅ 决议事项
1. 【决议内容】
2. ...（如无明确决议写"无"）

## 📝 待办事项
| 序号 | 待办内容 | 责任人 | 截止时间 |
|------|----------|--------|----------|
| 1 | 具体任务描述 | 从文本中提取，未提及写"待定" | 未提及写"待定" |

## 💡 补充说明
- 会议中提到但未形成结论的开放性问题或风险点（如无写"无"）

---

以下是需要整理的会议转写文本：

{{TEXT}}`

const defaultChunkSummaryPrompt = `# 角色
你是一位会议纪要助手，正在对一段较长会议的某个片段做关键信息提炼。

# 要求
- 输出 Markdown 格式
- 只输出该片段的信息，不要推测片段外的内容
- 语言精炼，去除口语化和重复表述
- 不要输出任何解释或前后文说明

# 输出结构

### 本段摘要
> 1-2 句话概括本片段讨论的核心内容。

### 讨论要点
- **要点**：结论或讨论结果

### 决议与待办
- 本片段中明确提到的决议或待办（如无写"无"）

### 关键信息
- 提及的人名、时间节点、数据指标等（如无写"无"）

---

以下是需要提炼的会议片段：

{{TEXT}}`

func (h *MeetingSummaryHandler) Validate(config json.RawMessage) error {
	if len(config) == 0 || string(config) == "null" || string(config) == "{}" {
		return nil // Will use fallback summarizer
	}
	var cfg MeetingSummaryConfig
	return json.Unmarshal(config, &cfg)
}

func (h *MeetingSummaryHandler) Execute(ctx context.Context, config json.RawMessage, inputText string, _ *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg MeetingSummaryConfig
	if len(config) > 0 && string(config) != "null" {
		_ = json.Unmarshal(config, &cfg)
	}

	// If LLM endpoint is configured, use it
	if cfg.Endpoint != "" && cfg.Model != "" {
		return h.executeLLMSummary(ctx, cfg, inputText)
	}

	// Fallback to existing summarizer
	if h.summarizer != nil {
		content, modelVersion, err := h.summarizer.Summarize(ctx, inputText)
		if err != nil {
			return inputText, nil, err
		}
		detail, _ := json.Marshal(map[string]string{
			"model_version": modelVersion,
			"source":        meetingSummaryBuiltinSource,
		})
		return content, detail, nil
	}

	return inputText, nil, fmt.Errorf("no summarizer configured")
}

// AppendSummaryToText appends the summary as a section at the bottom.
// This is a helper for post-processing workflows that want both text + summary.
func AppendSummaryToText(originalText, summary string) string {
	if strings.TrimSpace(summary) == "" {
		return originalText
	}
	return originalText + "\n\n---\n\n## 会议纪要\n\n" + summary
}

func (h *MeetingSummaryHandler) executeLLMSummary(ctx context.Context, cfg MeetingSummaryConfig, inputText string) (string, json.RawMessage, error) {
	trimmed := strings.TrimSpace(inputText)
	if trimmed == "" {
		return inputText, nil, fmt.Errorf("meeting summary input is empty")
	}

	finalPrompt := cfg.PromptTemplate
	if finalPrompt == "" {
		finalPrompt = defaultSummaryPrompt
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = meetingSummaryDefaultMaxTokens
	}

	if utf8.RuneCountInString(trimmed) <= meetingSummaryChunkMaxRunes {
		return h.executeSummaryPrompt(ctx, cfg, finalPrompt, trimmed, maxTokens, meetingSummaryDirectLLMSource, 1)
	}

	chunks := splitMeetingSummaryChunks(trimmed, meetingSummaryChunkMaxRunes)
	partialSummaries := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		partial, _, err := h.executeSummaryPrompt(ctx, cfg, defaultChunkSummaryPrompt, chunk, meetingSummaryChunkMaxTokens, meetingSummaryChunkedLLMSource, len(chunks))
		if err != nil {
			return inputText, nil, err
		}
		partialSummaries = append(partialSummaries, strings.TrimSpace(partial))
	}

	mergedInput := mergeChunkSummaries(partialSummaries)
	return h.executeSummaryPrompt(ctx, cfg, finalPrompt, mergedInput, maxTokens, meetingSummaryChunkedLLMSource, len(chunks))
}

func (h *MeetingSummaryHandler) executeSummaryPrompt(ctx context.Context, cfg MeetingSummaryConfig, prompt, inputText string, maxTokens int, source string, chunkCount int) (string, json.RawMessage, error) {
	llmCfg := LLMCorrectionConfig{
		Endpoint:       cfg.Endpoint,
		Model:          cfg.Model,
		APIKey:         cfg.APIKey,
		PromptTemplate: prompt,
		Temperature:    0.3,
		MaxTokens:      maxTokens,
		AllowMarkdown:  true,
	}
	llmConfigBytes, _ := json.Marshal(llmCfg)
	output, detail, err := h.llmHandler.Execute(ctx, llmConfigBytes, inputText, nil)
	if err != nil {
		return inputText, detail, err
	}

	var detailPayload map[string]any
	if len(detail) > 0 {
		_ = json.Unmarshal(detail, &detailPayload)
	}
	if detailPayload == nil {
		detailPayload = map[string]any{}
	}
	detailPayload["source"] = source
	detailPayload["chunk_count"] = chunkCount
	detailPayload["input_runes"] = utf8.RuneCountInString(strings.TrimSpace(inputText))
	mergedDetail, _ := json.Marshal(detailPayload)
	return output, mergedDetail, nil
}

func splitMeetingSummaryChunks(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		return []string{strings.TrimSpace(text)}
	}

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	chunks := make([]string, 0)
	currentLines := make([]string, 0)
	currentRunes := 0

	flush := func() {
		if len(currentLines) == 0 {
			return
		}
		chunks = append(chunks, strings.Join(currentLines, "\n"))
		currentLines = nil
		currentRunes = 0
	}

	appendOversizedLine := func(line string) {
		runes := []rune(line)
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
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lineRunes := utf8.RuneCountInString(line)
		if lineRunes > maxRunes {
			flush()
			appendOversizedLine(line)
			continue
		}

		addedRunes := lineRunes
		if len(currentLines) > 0 {
			addedRunes++
		}
		if currentRunes > 0 && currentRunes+addedRunes > maxRunes {
			flush()
		}
		currentLines = append(currentLines, line)
		currentRunes += addedRunes
	}
	flush()

	if len(chunks) == 0 {
		trimmed := strings.TrimSpace(text)
		if trimmed != "" {
			return []string{trimmed}
		}
	}
	return chunks
}

func mergeChunkSummaries(items []string) string {
	parts := make([]string, 0, len(items))
	for index, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("片段 %d 摘要：\n%s", index+1, trimmed))
	}
	return strings.Join(parts, "\n\n")
}
