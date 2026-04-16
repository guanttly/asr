package workflow

import (
	"context"
	"encoding/json"
	"testing"

	sensitivedomain "github.com/lgt/asr/internal/domain/sensitive"
)

type sensitiveDictRepoStub struct{}

func (r *sensitiveDictRepoStub) Create(_ context.Context, _ *sensitivedomain.Dict) error {
	panic("unexpected")
}
func (r *sensitiveDictRepoStub) GetByID(_ context.Context, id uint64) (*sensitivedomain.Dict, error) {
	return &sensitivedomain.Dict{ID: id, Name: "场景库"}, nil
}
func (r *sensitiveDictRepoStub) Update(_ context.Context, _ *sensitivedomain.Dict) error {
	panic("unexpected")
}
func (r *sensitiveDictRepoStub) Delete(_ context.Context, _ uint64) error { panic("unexpected") }
func (r *sensitiveDictRepoStub) List(_ context.Context, _, _ int) ([]*sensitivedomain.Dict, int64, error) {
	panic("unexpected")
}

type sensitiveEntryRepoStub struct {
	items []sensitivedomain.Entry
}

func (r *sensitiveEntryRepoStub) Create(_ context.Context, _ *sensitivedomain.Entry) error {
	panic("unexpected")
}
func (r *sensitiveEntryRepoStub) GetByID(_ context.Context, _ uint64) (*sensitivedomain.Entry, error) {
	panic("unexpected")
}
func (r *sensitiveEntryRepoStub) ListByDict(_ context.Context, _ uint64) ([]sensitivedomain.Entry, error) {
	panic("unexpected")
}
func (r *sensitiveEntryRepoStub) ListAppliedByDict(_ context.Context, _ uint64) ([]sensitivedomain.Entry, error) {
	return r.items, nil
}
func (r *sensitiveEntryRepoStub) Update(_ context.Context, _ *sensitivedomain.Entry) error {
	panic("unexpected")
}
func (r *sensitiveEntryRepoStub) Delete(_ context.Context, _ uint64) error { panic("unexpected") }

func TestSensitiveFilterHandlerMasksConfiguredWords(t *testing.T) {
	handler := NewSensitiveFilterHandler(&sensitiveDictRepoStub{}, &sensitiveEntryRepoStub{items: []sensitivedomain.Entry{{Word: "涉密项目", Enabled: true}}})
	config := json.RawMessage(`{"dict_id":2,"words":["张三"],"replacement":"***"}`)

	output, detail, err := handler.Execute(context.Background(), config, "张三参与了涉密项目讨论", nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if output != "***参与了***讨论" {
		t.Fatalf("unexpected output: %q", output)
	}

	var payload map[string]any
	if err := json.Unmarshal(detail, &payload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if payload["replacement"] != "***" {
		t.Fatalf("expected replacement marker in detail, got %+v", payload["replacement"])
	}
	matchedWords, ok := payload["matched_words"].([]any)
	if !ok || len(matchedWords) != 2 {
		t.Fatalf("expected matched_words detail, got %+v", payload["matched_words"])
	}
	foundInline := false
	foundDict := false
	for _, item := range matchedWords {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if entry["word"] == "张三" {
			foundInline = true
			sources, _ := entry["sources"].([]any)
			if len(sources) == 0 || sources[0] != "兼容内联词" {
				t.Fatalf("expected inline source for 张三, got %+v", entry["sources"])
			}
		}
		if entry["word"] == "涉密项目" {
			foundDict = true
			sources, _ := entry["sources"].([]any)
			if len(sources) == 0 || sources[0] != "场景库" {
				t.Fatalf("expected dict source for 涉密项目, got %+v", entry["sources"])
			}
		}
	}
	if !foundInline || !foundDict {
		t.Fatalf("expected both inline and dict matched words, got %+v", matchedWords)
	}
}
