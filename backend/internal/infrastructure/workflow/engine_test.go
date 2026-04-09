package workflow

import (
	"context"
	"testing"

	domain "github.com/lgt/asr/internal/domain/workflow"
	"go.uber.org/zap"
)

func TestTestNodeRejectsSourceNodes(t *testing.T) {
	engine := NewEngine(zap.NewNop())

	result, err := engine.TestNode(context.Background(), domain.NodeBatchASR, []byte(`{}`), "示例文本")
	if err != nil {
		t.Fatalf("TestNode returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result for source node test")
	}
	if result.Error == "" {
		t.Fatal("expected source node test to return clear rejection message")
	}
	if result.OutputText != "" {
		t.Fatalf("expected source node test to avoid output text, got %q", result.OutputText)
	}
}
