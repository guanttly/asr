package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	domain "github.com/lgt/asr/internal/domain/workflow"
	wfengine "github.com/lgt/asr/internal/infrastructure/workflow"
)

const defaultLLMCorrectionPrompt = `你是一个专业的语音转写文本校对助手。请只修正语音识别造成的错别字、同音误识别、标点和明显语序问题，保持原意、语气、人名、数字和专业术语不变。

要求：
1. 只输出纠错后的正文，不要解释、不要标题、不要列表。
2. 如果原文为空或只有空白，直接输出空字符串，不要补充提示语。
3. 无法确定的内容保持原样，不要编造。

原文：
{{TEXT}}`

func builtinNodeDefaultConfig(nodeType domain.NodeType) map[string]any {
	switch nodeType {
	case domain.NodeTermCorrection:
		return map[string]any{"dict_id": 0}
	case domain.NodeFillerFilter:
		return map[string]any{"dict_id": 0, "filter_words": []string{}, "custom_words": []string{}}
	case domain.NodeSensitiveFilter:
		return map[string]any{"dict_id": 0, "custom_words": []string{}, "replacement": "[已过滤]"}
	case domain.NodeLLMCorrection:
		return map[string]any{"endpoint": "", "model": "", "api_key": "", "prompt_template": defaultLLMCorrectionPrompt, "temperature": 0.3, "max_tokens": 4096, "allow_markdown": false}
	case domain.NodeVoiceWake:
		return map[string]any{"wake_words": []string{"你好小鲨"}, "homophone_words": []string{"你好小沙", "你好小莎", "你好小善"}}
	case domain.NodeVoiceIntent:
		return map[string]any{"enable_llm": false, "endpoint": "", "model": "", "api_key": "", "prompt_template": "", "extra_prompt": "", "temperature": 0.0, "max_tokens": 512, "include_base": true, "dict_ids": []uint64{}}
	case domain.NodeSpeakerDiarize:
		return map[string]any{"service_url": "", "enable_voiceprint_match": false, "fail_on_error": false}
	case domain.NodeMeetingSummary:
		return map[string]any{"endpoint": "", "model": "", "api_key": "", "prompt_template": wfengine.DefaultMeetingSummaryPrompt, "chunk_prompt_template": wfengine.DefaultMeetingSummaryChunkPrompt, "output_format": "markdown", "max_tokens": 100000}
	case domain.NodeCustomRegex:
		return map[string]any{"rules": []map[string]any{{"pattern": "", "replacement": "", "enabled": true}}}
	default:
		return map[string]any{}
	}
}

func mustMarshalNodeConfig(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func parseNodeConfigMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" {
		return map[string]any{}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return map[string]any{}, nil
	}
	return payload, nil
}

func mergeExplicitConfigMaps(base map[string]any, override map[string]any) map[string]any {
	result := cloneConfigMap(base)
	for key, value := range override {
		baseValue, hasBase := result[key]
		overrideMap, overrideIsMap := value.(map[string]any)
		baseMap, baseIsMap := baseValue.(map[string]any)
		if hasBase && overrideIsMap && baseIsMap {
			result[key] = mergeExplicitConfigMaps(baseMap, overrideMap)
			continue
		}
		result[key] = cloneConfigValue(value)
	}
	return result
}

func mergeNodeOverrideMaps(base map[string]any, override map[string]any) map[string]any {
	result := cloneConfigMap(base)
	for key, value := range override {
		if !shouldApplyNodeOverride(value) {
			continue
		}

		baseValue, hasBase := result[key]
		overrideMap, overrideIsMap := value.(map[string]any)
		baseMap, baseIsMap := baseValue.(map[string]any)
		if hasBase && overrideIsMap && baseIsMap {
			result[key] = mergeNodeOverrideMaps(baseMap, overrideMap)
			continue
		}
		result[key] = cloneConfigValue(value)
	}
	return result
}

func shouldApplyNodeOverride(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return true
	case float64:
		return typed != 0
	case float32:
		return typed != 0
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case int32:
		return typed != 0
	case uint64:
		return typed != 0
	case uint32:
		return typed != 0
	case []any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func cloneConfigMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = cloneConfigValue(value)
	}
	return result
}

func cloneConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneConfigMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i := range typed {
			cloned[i] = cloneConfigValue(typed[i])
		}
		return cloned
	case []string:
		cloned := make([]string, len(typed))
		copy(cloned, typed)
		return cloned
	default:
		return typed
	}
}

func (s *Service) resolveGlobalNodeDefault(ctx context.Context, nodeType domain.NodeType) (json.RawMessage, error) {
	base := builtinNodeDefaultConfig(nodeType)
	result := cloneConfigMap(base)
	if s.nodeDefaultRepo == nil {
		normalizeResolvedNodeDefault(nodeType, result)
		return mustMarshalNodeConfig(result), nil
	}

	item, err := s.nodeDefaultRepo.GetByType(ctx, nodeType)
	if err != nil {
		return nil, err
	}
	if item == nil || strings.TrimSpace(item.Config) == "" {
		normalizeResolvedNodeDefault(nodeType, result)
		return mustMarshalNodeConfig(result), nil
	}

	override, err := parseNodeConfigMap(json.RawMessage(item.Config))
	if err != nil {
		return nil, fmt.Errorf("invalid stored default config for %s: %w", nodeType, err)
	}
	result = mergeExplicitConfigMaps(result, override)
	normalizeResolvedNodeDefault(nodeType, result)
	return mustMarshalNodeConfig(result), nil
}

func normalizeResolvedNodeDefault(nodeType domain.NodeType, config map[string]any) {
	if nodeType == domain.NodeLLMCorrection {
		promptTemplate, _ := config["prompt_template"].(string)
		if strings.TrimSpace(promptTemplate) == "" {
			config["prompt_template"] = defaultLLMCorrectionPrompt
		}
		return
	}

	if nodeType == domain.NodeMeetingSummary {
		promptTemplate, _ := config["prompt_template"].(string)
		if strings.TrimSpace(promptTemplate) == "" {
			config["prompt_template"] = wfengine.DefaultMeetingSummaryPrompt
		}
		chunkPromptTemplate, _ := config["chunk_prompt_template"].(string)
		if strings.TrimSpace(chunkPromptTemplate) == "" {
			config["chunk_prompt_template"] = wfengine.DefaultMeetingSummaryChunkPrompt
		}
	}
}

func (s *Service) resolveNodeConfig(ctx context.Context, nodeType domain.NodeType, raw json.RawMessage) (json.RawMessage, error) {
	defaults, err := s.resolveGlobalNodeDefault(ctx, nodeType)
	if err != nil {
		return nil, err
	}

	defaultMap, err := parseNodeConfigMap(defaults)
	if err != nil {
		return nil, err
	}
	overrideMap, err := parseNodeConfigMap(raw)
	if err != nil {
		return nil, err
	}
	return mustMarshalNodeConfig(mergeNodeOverrideMaps(defaultMap, overrideMap)), nil
}
