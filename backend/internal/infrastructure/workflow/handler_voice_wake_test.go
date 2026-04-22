package workflow

import (
	"context"
	"encoding/json"
	"testing"
)

func TestVoiceWakeHandlerMapsHomophoneToCanonicalWakeWord(t *testing.T) {
	handler := NewVoiceWakeHandler()
	config := json.RawMessage(`{"wake_words":["小鲨小鲨"],"homophone_words":["小沙小沙","小莎小莎"]}`)

	output, detail, err := handler.Execute(context.Background(), config, "小沙小沙 切换到会议模式", nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var result VoiceWakeResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !result.WakeMatched {
		t.Fatal("expected homophone wake word to match")
	}
	if result.WakeWord != "小鲨小鲨" {
		t.Fatalf("expected canonical wake word, got %q", result.WakeWord)
	}
	if result.WakeAlias != "小沙小沙" {
		t.Fatalf("expected matched alias to be preserved, got %q", result.WakeAlias)
	}
	if result.Residue != "切换到会议模式" {
		t.Fatalf("expected residue to keep trailing command, got %q", result.Residue)
	}

	var payload map[string]any
	if err := json.Unmarshal(detail, &payload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if payload["wake_word"] != "小鲨小鲨" {
		t.Fatalf("expected detail wake_word to be canonical, got %+v", payload["wake_word"])
	}
	if payload["wake_alias"] != "小沙小沙" {
		t.Fatalf("expected detail wake_alias to keep homophone alias, got %+v", payload["wake_alias"])
	}
	if payload["residue"] != "切换到会议模式" {
		t.Fatalf("expected detail residue to keep trailing command, got %+v", payload["residue"])
	}
}
