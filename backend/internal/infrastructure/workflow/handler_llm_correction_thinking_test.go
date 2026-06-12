package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLLMCorrectionHandlerStripsThinkBlockJSON reproduces bug 14853: when the
// configured model is a qwen3 "thinking" model served without a reasoning
// parser, the <think>...</think> block is emitted inline in message.content.
// The handler must strip it so the corrected text is usable.
func TestLLMCorrectionHandlerStripsThinkBlockJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"<think>用户想要纠错这段文本，我需要把错别字改正。\n这里应该是“患者”。</think>患者主诉头痛三天。"}}],"model":"qwen3-4b","usage":{"prompt_tokens":40,"completion_tokens":30}}`))
	}))
	defer server.Close()

	handler := NewLLMCorrectionHandler()
	configBytes := []byte(`{
		"endpoint": "` + server.URL + `",
		"model": "qwen3-4b",
		"prompt_template": "请纠错：{{TEXT}}",
		"temperature": 0.3,
		"max_tokens": 512
	}`)

	output, _, err := handler.Execute(context.Background(), configBytes, "患者主诉头痛三天", nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(output, "<think>") || strings.Contains(output, "</think>") || strings.Contains(output, "用户想要纠错") {
		t.Fatalf("expected think block stripped, got %q", output)
	}
	if output != "患者主诉头痛三天。" {
		t.Fatalf("expected clean corrected text, got %q", output)
	}
}

// TestLLMCorrectionHandlerStripsThinkBlockStream reproduces bug 14853 for the
// streaming path used by the node test feature: the <think> block is split
// across SSE deltas and must not leak into the final output.
func TestLLMCorrectionHandlerStripsThinkBlockStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"<think>让我\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"思考一下</think>\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"患者主诉\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"头痛三天。\"}}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":8}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	handler := NewLLMCorrectionHandler()
	configBytes := []byte(`{
		"endpoint": "` + server.URL + `",
		"model": "qwen3-4b",
		"prompt_template": "请纠错：{{TEXT}}",
		"temperature": 0.3,
		"max_tokens": 512
	}`)

	var lastDelta string
	output, _, err := handler.ExecuteStream(context.Background(), configBytes, "原始文本", nil, func(event *NodeStreamEvent) error {
		if event != nil && event.Type == NodeStreamEventDelta {
			lastDelta = event.OutputText
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}
	if strings.Contains(output, "<think>") || strings.Contains(output, "</think>") || strings.Contains(output, "思考一下") {
		t.Fatalf("expected think block stripped from stream output, got %q", output)
	}
	if output != "患者主诉头痛三天。" {
		t.Fatalf("expected clean streamed text, got %q (lastDelta=%q)", output, lastDelta)
	}
}

// TestLLMCorrectionHandlerIgnoresReasoningContentField verifies that the
// separate reasoning_content channel (DashScope / vLLM-with-parser style) is
// not mistaken for the answer; only content is kept.
func TestLLMCorrectionHandlerIgnoresReasoningContentField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"让我想想这段话\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"应该怎么改\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"患者主诉头痛三天。\"}}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":8}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	handler := NewLLMCorrectionHandler()
	configBytes := []byte(`{
		"endpoint": "` + server.URL + `",
		"model": "qwen3-4b",
		"prompt_template": "请纠错：{{TEXT}}",
		"temperature": 0.3,
		"max_tokens": 512
	}`)

	output, _, err := handler.ExecuteStream(context.Background(), configBytes, "原始文本", nil, nil)
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}
	if strings.Contains(output, "让我想想") || strings.Contains(output, "应该怎么改") {
		t.Fatalf("expected reasoning_content ignored, got %q", output)
	}
	if output != "患者主诉头痛三天。" {
		t.Fatalf("expected only content kept, got %q", output)
	}
}
