package workflow

import (
	"context"
	"encoding/json"
	"testing"

	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
	voicecommand "github.com/lgt/asr/internal/domain/voicecommand"
)

type openSkillInvokerStub struct {
	result *openplatformdomain.SkillInvokeResult
	err    error
	called bool
	userID uint64
	taskID uint64
}

func (s *openSkillInvokerStub) MatchAndInvokeSkill(_ context.Context, ownerUserID uint64, _ string, taskID, _ uint64) (*openplatformdomain.SkillInvokeResult, error) {
	s.called = true
	s.userID = ownerUserID
	s.taskID = taskID
	return s.result, s.err
}

type voiceIntentDictRepoStub struct {
	items []*voicecommand.Dict
}

func (r *voiceIntentDictRepoStub) Create(_ context.Context, _ *voicecommand.Dict) error {
	panic("unexpected")
}
func (r *voiceIntentDictRepoStub) GetByID(_ context.Context, _ uint64) (*voicecommand.Dict, error) {
	panic("unexpected")
}
func (r *voiceIntentDictRepoStub) Update(_ context.Context, _ *voicecommand.Dict) error {
	panic("unexpected")
}
func (r *voiceIntentDictRepoStub) Delete(_ context.Context, _ uint64) error {
	panic("unexpected")
}
func (r *voiceIntentDictRepoStub) List(_ context.Context, _, _ int) ([]*voicecommand.Dict, int64, error) {
	return r.items, int64(len(r.items)), nil
}
func (r *voiceIntentDictRepoStub) ListByIDs(_ context.Context, _ []uint64) ([]*voicecommand.Dict, error) {
	return r.items, nil
}

type voiceIntentEntryRepoStub struct {
	items []voicecommand.Entry
}

func (r *voiceIntentEntryRepoStub) Create(_ context.Context, _ *voicecommand.Entry) error {
	panic("unexpected")
}
func (r *voiceIntentEntryRepoStub) GetByID(_ context.Context, _ uint64) (*voicecommand.Entry, error) {
	panic("unexpected")
}
func (r *voiceIntentEntryRepoStub) ListByDict(_ context.Context, _ uint64) ([]voicecommand.Entry, error) {
	panic("unexpected")
}
func (r *voiceIntentEntryRepoStub) ListByDicts(_ context.Context, dictIDs []uint64) ([]voicecommand.Entry, error) {
	selected := map[uint64]struct{}{}
	for _, id := range dictIDs {
		selected[id] = struct{}{}
	}
	items := make([]voicecommand.Entry, 0, len(r.items))
	for _, item := range r.items {
		if _, ok := selected[item.DictID]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *voiceIntentEntryRepoStub) Update(_ context.Context, _ *voicecommand.Entry) error {
	panic("unexpected")
}
func (r *voiceIntentEntryRepoStub) Delete(_ context.Context, _ uint64) error {
	panic("unexpected")
}

func TestVoiceIntentValidateAllowsCatalogOnlyMode(t *testing.T) {
	handler := NewVoiceIntentHandler(nil, nil)
	if err := handler.Validate(json.RawMessage(`{"enable_llm":false,"include_base":true,"dict_ids":[]}`)); err != nil {
		t.Fatalf("expected catalog-only config to validate, got %v", err)
	}
}

func TestVoiceIntentExecuteMatchesCatalogWhenLLMDisabled(t *testing.T) {
	handler := NewVoiceIntentHandler(
		&voiceIntentDictRepoStub{items: []*voicecommand.Dict{{ID: 1, Name: "场景模式", GroupKey: "scene_mode", IsBase: true}}},
		&voiceIntentEntryRepoStub{items: []voicecommand.Entry{{ID: 2, DictID: 1, Intent: "scene_meeting_switch", Label: "会议模式", Utterances: []string{"会议模式", "切换到会议模式"}, Enabled: true}}},
	)

	output, detail, err := handler.Execute(context.Background(), json.RawMessage(`{"enable_llm":false,"include_base":true,"dict_ids":[]}`), "切换到会议模式", nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["matched"] != true {
		t.Fatalf("expected matched result, got %+v", result)
	}
	if result["intent"] != "scene_meeting_switch" {
		t.Fatalf("expected scene_meeting_switch intent, got %+v", result["intent"])
	}
	if result["group_key"] != "scene_mode" {
		t.Fatalf("expected scene_mode group key, got %+v", result["group_key"])
	}

	var payload map[string]any
	if err := json.Unmarshal(detail, &payload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if payload["match_mode"] != "catalog" {
		t.Fatalf("expected catalog match mode, got %+v", payload["match_mode"])
	}
	if payload["command_id"] != float64(2) {
		t.Fatalf("expected command id 2, got %+v", payload["command_id"])
	}
}

func TestVoiceIntentExecuteFallsBackToOpenSkill(t *testing.T) {
	invoker := &openSkillInvokerStub{result: &openplatformdomain.SkillInvokeResult{
		SkillID:        "skl_voice_assistant",
		SkillName:      "book_meeting_room",
		MatchedPattern: "预订会议室",
		Status:         openplatformdomain.InvocationStatusSuccess,
		ResponseJSON:   `{"accepted":true}`,
	}}
	handler := NewVoiceIntentHandler(nil, nil)
	handler.SetOpenSkillInvoker(invoker)

	output, detail, err := handler.Execute(context.Background(), json.RawMessage(`{"enable_llm":false,"include_base":false,"dict_ids":[]}`), "请帮我预订会议室", &ExecutionMeta{
		UserID: openplatformdomain.OwnerUserIDForApp(9),
		TaskID: 42,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !invoker.called {
		t.Fatal("expected open skill invoker to be called")
	}
	if invoker.userID != openplatformdomain.OwnerUserIDForApp(9) || invoker.taskID != 42 {
		t.Fatalf("unexpected invocation context: user=%d task=%d", invoker.userID, invoker.taskID)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if result["matched"] != true || result["group_key"] != "open_skill" {
		t.Fatalf("expected open_skill match, got %+v", result)
	}

	var payload map[string]any
	if err := json.Unmarshal(detail, &payload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if payload["match_mode"] != "open_skill" {
		t.Fatalf("expected open_skill match mode, got %+v", payload["match_mode"])
	}
	callbackResponse, ok := payload["callback_response"].(map[string]any)
	if !ok || callbackResponse["accepted"] != true {
		t.Fatalf("expected callback response payload, got %+v", payload["callback_response"])
	}
}
