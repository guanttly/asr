package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
	voicecommand "github.com/lgt/asr/internal/domain/voicecommand"
	voiceintent "github.com/lgt/asr/internal/infrastructure/workflow/voiceintent"
)

type OpenSkillInvoker interface {
	MatchAndInvokeSkill(ctx context.Context, ownerUserID uint64, utterance string, taskID, meetingID uint64) (*openplatformdomain.SkillInvokeResult, error)
}

type VoiceIntentConfig struct {
	EnableLLM      bool     `json:"enable_llm"`
	Endpoint       string   `json:"endpoint"`
	Model          string   `json:"model"`
	APIKey         string   `json:"api_key,omitempty"`
	PromptTemplate string   `json:"prompt_template"`
	ExtraPrompt    string   `json:"extra_prompt"`
	Temperature    float64  `json:"temperature"`
	MaxTokens      int      `json:"max_tokens"`
	IncludeBase    bool     `json:"include_base"`
	DictIDs        []uint64 `json:"dict_ids"`
}

type VoiceIntentHandler struct {
	httpClient       *http.Client
	dictRepo         voicecommand.DictRepository
	entryRepo        voicecommand.EntryRepository
	openSkillInvoker OpenSkillInvoker
}

func NewVoiceIntentHandler(dictRepo voicecommand.DictRepository, entryRepo voicecommand.EntryRepository) *VoiceIntentHandler {
	return &VoiceIntentHandler{httpClient: &http.Client{}, dictRepo: dictRepo, entryRepo: entryRepo}
}

func (h *VoiceIntentHandler) SetOpenSkillInvoker(invoker OpenSkillInvoker) {
	h.openSkillInvoker = invoker
}

func (h *VoiceIntentHandler) Validate(config json.RawMessage) error {
	var cfg VoiceIntentConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid voice_intent config: %w", err)
	}
	if !cfg.EnableLLM {
		return nil
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return fmt.Errorf("endpoint is required")
	}
	if err := validateOpenAIChatEndpoint(cfg.Endpoint); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

func (h *VoiceIntentHandler) Execute(ctx context.Context, config json.RawMessage, inputText string, meta *ExecutionMeta) (string, json.RawMessage, error) {
	var cfg VoiceIntentConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return inputText, nil, err
	}
	var wakeResult *VoiceWakeResult
	if parsedWakeResult, ok := parseVoiceWakeResult(inputText); ok {
		wakeResult = &parsedWakeResult
		if !parsedWakeResult.WakeMatched {
			result := voiceintent.Result{
				WakeMatched: false,
				Reason:      fallbackReason(parsedWakeResult.Reason, "未命中唤醒词"),
			}
			output, _ := json.Marshal(result)
			detail, _ := json.Marshal(map[string]any{
				"wake_matched": false,
				"reason":       result.Reason,
			})
			return string(output), detail, nil
		}
		if strings.TrimSpace(parsedWakeResult.Residue) == "" {
			result := voiceintent.Result{
				WakeMatched: true,
				WakeWord:    parsedWakeResult.WakeWord,
				WakeAlias:   parsedWakeResult.WakeAlias,
				Reason:      fallbackReason(parsedWakeResult.Reason, "已命中唤醒词，等待后续指令"),
			}
			output, _ := json.Marshal(result)
			detail, _ := json.Marshal(map[string]any{
				"wake_matched": true,
				"wake_word":    parsedWakeResult.WakeWord,
				"wake_alias":   parsedWakeResult.WakeAlias,
				"reason":       result.Reason,
			})
			return string(output), detail, nil
		}
		inputText = parsedWakeResult.Residue
	}
	dicts, entries, catalog, err := h.resolveCatalog(ctx, cfg)
	if err != nil {
		return inputText, nil, err
	}
	if !cfg.EnableLLM || len(catalog.Commands) == 0 {
		result, detail := buildCatalogMatchResult(inputText, dicts, entries, catalog)
		result, detail, err = h.applyOpenSkillFallback(ctx, inputText, meta, result, detail)
		if err != nil {
			return inputText, nil, err
		}
		if wakeResult != nil {
			result.WakeMatched = wakeResult.WakeMatched
			result.WakeWord = wakeResult.WakeWord
			result.WakeAlias = wakeResult.WakeAlias
			detail, _ = json.Marshal(map[string]any{
				"match_mode":    "catalog",
				"dict_ids":      catalog.DictIDs,
				"group_keys":    catalog.GroupKeys,
				"dict_count":    len(dicts),
				"command_count": countEnabledEntries(entries),
				"wake_matched":  result.WakeMatched,
				"wake_word":     result.WakeWord,
				"wake_alias":    result.WakeAlias,
				"matched":       result.Matched,
				"intent":        result.Intent,
				"group_key":     result.GroupKey,
				"command_id":    result.CommandID,
				"confidence":    result.Confidence,
				"reason":        result.Reason,
				"raw_output":    result.RawOutput,
			})
		}
		output, err := json.Marshal(result)
		if err != nil {
			return inputText, nil, err
		}
		return string(output), detail, nil
	}
	endpoint, err := normalizeOpenAIChatEndpoint(cfg.Endpoint)
	if err != nil {
		return inputText, nil, err
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	temperature := cfg.Temperature
	if temperature < 0 {
		temperature = 0
	}
	prompt := enforcePlainTextOutput(voiceintent.BuildPrompt(cfg.PromptTemplate, cfg.ExtraPrompt, inputText, catalog))
	respBody, statusCode, err := h.executeChatRequest(ctx, endpoint, cfg, prompt, temperature, maxTokens)
	if err != nil {
		return inputText, nil, err
	}
	if statusCode != http.StatusOK {
		return inputText, nil, fmt.Errorf("voice_intent request to %s returned status %d: %s", endpoint, statusCode, string(respBody))
	}
	result, detail, err := h.parseResult(respBody, dicts, entries, catalog)
	if err != nil {
		return inputText, nil, err
	}
	result, detail, err = h.applyOpenSkillFallback(ctx, inputText, meta, result, detail)
	if err != nil {
		return inputText, nil, err
	}
	if wakeResult != nil {
		result.WakeMatched = wakeResult.WakeMatched
		result.WakeWord = wakeResult.WakeWord
		result.WakeAlias = wakeResult.WakeAlias
		detail, _ = json.Marshal(map[string]any{
			"wake_matched": result.WakeMatched,
			"wake_word":    result.WakeWord,
			"wake_alias":   result.WakeAlias,
			"matched":      result.Matched,
			"intent":       result.Intent,
			"group_key":    result.GroupKey,
			"command_id":   result.CommandID,
			"confidence":   result.Confidence,
			"reason":       result.Reason,
			"raw_output":   result.RawOutput,
		})
	}
	output, err := json.Marshal(result)
	if err != nil {
		return inputText, nil, err
	}
	return string(output), detail, nil
}

func (h *VoiceIntentHandler) resolveCatalog(ctx context.Context, cfg VoiceIntentConfig) ([]*voicecommand.Dict, []voicecommand.Entry, voiceintent.Catalog, error) {
	if h.dictRepo == nil || h.entryRepo == nil {
		if h.openSkillInvoker != nil {
			return nil, nil, voiceintent.Catalog{}, nil
		}
		return nil, nil, voiceintent.Catalog{}, fmt.Errorf("voice command repositories are not configured")
	}
	dicts, _, err := h.dictRepo.List(ctx, 0, 1000)
	if err != nil {
		return nil, nil, voiceintent.Catalog{}, err
	}
	selectedDicts := make([]*voicecommand.Dict, 0, len(dicts))
	selectedDictIDs := make([]uint64, 0, len(cfg.DictIDs)+4)
	selectedSet := map[uint64]struct{}{}
	for _, id := range cfg.DictIDs {
		if id > 0 {
			selectedSet[id] = struct{}{}
		}
	}
	for _, dict := range dicts {
		if dict == nil {
			continue
		}
		if dict.IsBase && cfg.IncludeBase {
			selectedDicts = append(selectedDicts, dict)
			selectedDictIDs = append(selectedDictIDs, dict.ID)
			continue
		}
		if _, ok := selectedSet[dict.ID]; ok {
			selectedDicts = append(selectedDicts, dict)
			selectedDictIDs = append(selectedDictIDs, dict.ID)
		}
	}
	entries, err := h.entryRepo.ListByDicts(ctx, selectedDictIDs)
	if err != nil {
		return nil, nil, voiceintent.Catalog{}, err
	}
	catalog, err := voiceintent.BuildCatalog(selectedDicts, entries, cfg.DictIDs, cfg.IncludeBase)
	if err != nil {
		if h.openSkillInvoker != nil {
			return selectedDicts, entries, voiceintent.Catalog{}, nil
		}
		return nil, nil, voiceintent.Catalog{}, err
	}
	return selectedDicts, entries, catalog, nil
}

func (h *VoiceIntentHandler) applyOpenSkillFallback(ctx context.Context, inputText string, meta *ExecutionMeta, result voiceintent.Result, detail json.RawMessage) (voiceintent.Result, json.RawMessage, error) {
	if result.Matched || h.openSkillInvoker == nil || meta == nil || strings.TrimSpace(inputText) == "" {
		return result, detail, nil
	}
	invocation, err := h.openSkillInvoker.MatchAndInvokeSkill(ctx, meta.UserID, inputText, meta.TaskID, meta.MeetingID)
	if err != nil {
		return result, nil, err
	}
	if invocation == nil {
		return result, detail, nil
	}
	result = voiceintent.Result{
		Matched:    true,
		Intent:     invocation.SkillName,
		GroupKey:   "open_skill",
		Confidence: 1,
		Reason:     openSkillInvokeReason(invocation),
		RawOutput:  invocation.MatchedPattern,
	}
	return result, openSkillInvokeDetail(invocation), nil
}

func openSkillInvokeDetail(result *openplatformdomain.SkillInvokeResult) json.RawMessage {
	if result == nil {
		return nil
	}
	payload := map[string]any{
		"match_mode":        "open_skill",
		"matched":           true,
		"group_key":         "open_skill",
		"intent":            result.SkillName,
		"skill_id":          result.SkillID,
		"skill_name":        result.SkillName,
		"matched_pattern":   result.MatchedPattern,
		"invocation_status": result.Status,
		"error_message":     result.ErrorMessage,
	}
	if result.HTTPStatus != nil {
		payload["http_status"] = *result.HTTPStatus
	}
	if strings.TrimSpace(result.ResponseJSON) != "" {
		var response any
		if err := json.Unmarshal([]byte(result.ResponseJSON), &response); err == nil {
			payload["callback_response"] = response
		} else {
			payload["callback_response_raw"] = result.ResponseJSON
		}
	}
	detail, _ := json.Marshal(payload)
	return detail
}

func openSkillInvokeReason(result *openplatformdomain.SkillInvokeResult) string {
	if result == nil {
		return "未命中开放技能"
	}
	name := strings.TrimSpace(result.SkillName)
	if name == "" {
		name = strings.TrimSpace(result.SkillID)
	}
	switch result.Status {
	case openplatformdomain.InvocationStatusSuccess:
		return fmt.Sprintf("已触发开放技能：%s", name)
	case openplatformdomain.InvocationStatusTimeout:
		return fmt.Sprintf("已命中开放技能但回调超时：%s", name)
	case openplatformdomain.InvocationStatusSignedRejected:
		return fmt.Sprintf("已命中开放技能但签名被拒绝：%s", name)
	default:
		return fmt.Sprintf("已命中开放技能但回调失败：%s", name)
	}
}

func (h *VoiceIntentHandler) executeChatRequest(ctx context.Context, endpoint string, cfg VoiceIntentConfig, prompt string, temperature float64, maxTokens int) ([]byte, int, error) {
	reqBody := map[string]any{
		"model":       cfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
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
	if strings.TrimSpace(cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("voice_intent request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read voice_intent response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

func (h *VoiceIntentHandler) parseResult(respBody []byte, dicts []*voicecommand.Dict, entries []voicecommand.Entry, catalog voiceintent.Catalog) (voiceintent.Result, json.RawMessage, error) {
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
		return voiceintent.Result{}, nil, fmt.Errorf("failed to parse voice_intent response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return voiceintent.Result{}, nil, fmt.Errorf("voice_intent returned no choices")
	}
	rawOutput := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	result, ok := voiceintent.Parse(rawOutput)
	if !ok {
		return voiceintent.Result{}, nil, fmt.Errorf("voice_intent returned invalid JSON: %s", rawOutput)
	}
	entryCount := 0
	for _, entry := range entries {
		if entry.Enabled {
			entryCount++
		}
	}
	detail, _ := json.Marshal(map[string]any{
		"match_mode":        "llm",
		"model":             chatResp.Model,
		"prompt_tokens":     chatResp.Usage.PromptTokens,
		"completion_tokens": chatResp.Usage.CompletionTokens,
		"dict_ids":          catalog.DictIDs,
		"group_keys":        catalog.GroupKeys,
		"dict_count":        len(dicts),
		"command_count":     entryCount,
		"matched":           result.Matched,
		"wake_matched":      result.WakeMatched,
		"wake_word":         result.WakeWord,
		"wake_alias":        result.WakeAlias,
		"intent":            result.Intent,
		"group_key":         result.GroupKey,
		"command_id":        result.CommandID,
		"confidence":        result.Confidence,
		"reason":            result.Reason,
		"raw_output":        result.RawOutput,
	})
	return result, detail, nil
}

func countEnabledEntries(entries []voicecommand.Entry) int {
	count := 0
	for _, entry := range entries {
		if entry.Enabled {
			count++
		}
	}
	return count
}

func fallbackReason(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func buildCatalogMatchResult(inputText string, dicts []*voicecommand.Dict, entries []voicecommand.Entry, catalog voiceintent.Catalog) (voiceintent.Result, json.RawMessage) {
	result := voiceintent.MatchCatalog(inputText, catalog)
	detail, _ := json.Marshal(map[string]any{
		"match_mode":    "catalog",
		"dict_ids":      catalog.DictIDs,
		"group_keys":    catalog.GroupKeys,
		"dict_count":    len(dicts),
		"command_count": countEnabledEntries(entries),
		"matched":       result.Matched,
		"wake_matched":  result.WakeMatched,
		"wake_word":     result.WakeWord,
		"wake_alias":    result.WakeAlias,
		"intent":        result.Intent,
		"group_key":     result.GroupKey,
		"command_id":    result.CommandID,
		"confidence":    result.Confidence,
		"reason":        result.Reason,
		"raw_output":    result.RawOutput,
	})
	return result, detail
}
