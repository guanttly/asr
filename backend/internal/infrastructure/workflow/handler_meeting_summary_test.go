package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSplitMeetingSummaryChunksKeepsLinesAndLimitsChunkSize(t *testing.T) {
	text := strings.Join([]string{
		"Speaker A：第一段内容",
		"Speaker B：第二段内容",
		"Speaker A：第三段内容",
		"Speaker C：第四段内容",
	}, "\n")

	chunks := splitMeetingSummaryChunks(text, 20)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 20 {
			t.Fatalf("expected chunk rune length <= 20, got %d: %q", len([]rune(chunk)), chunk)
		}
	}
}

func TestMeetingSummaryHandlerChunksLongInputBeforeFinalSummary(t *testing.T) {
	var requestCount atomic.Int32
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Messages) == 0 {
			t.Fatal("expected at least one message")
		}
		prompts = append(prompts, payload.Messages[0].Content)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"阶段摘要"}}],"model":"qwen3-4b","usage":{"prompt_tokens":100,"completion_tokens":50}}`))
	}))
	defer server.Close()

	handler := NewMeetingSummaryHandler(nil)
	inputLines := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		inputLines = append(inputLines, "Speaker A："+strings.Repeat("会议内容", 140))
	}
	inputText := strings.Join(inputLines, "\n")

	configBytes := []byte(`{"endpoint":"` + server.URL + `","model":"qwen3-4b"}`)
	output, detail, err := handler.Execute(context.Background(), configBytes, inputText, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.TrimSpace(output) == "" {
		t.Fatal("expected non-empty summary output")
	}
	if requestCount.Load() < 3 {
		t.Fatalf("expected chunked summarization to issue multiple requests, got %d", requestCount.Load())
	}
	lastPrompt := prompts[len(prompts)-1]
	if !strings.Contains(lastPrompt, "片段 1 摘要") {
		t.Fatalf("expected final prompt to merge chunk summaries, got %q", lastPrompt)
	}
	var detailPayload map[string]any
	if err := json.Unmarshal(detail, &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if detailPayload["source"] != meetingSummaryChunkedLLMSource {
		t.Fatalf("expected source %q, got %+v", meetingSummaryChunkedLLMSource, detailPayload["source"])
	}
	if detailPayload["chunk_count"].(float64) < 2 {
		t.Fatalf("expected chunk_count >= 2, got %+v", detailPayload["chunk_count"])
	}
	chunkOutputs, ok := detailPayload["chunk_outputs"].([]any)
	if !ok || len(chunkOutputs) == 0 {
		t.Fatalf("expected chunk_outputs to be present, got %+v", detailPayload["chunk_outputs"])
	}
	firstChunk, ok := chunkOutputs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first chunk output payload, got %+v", chunkOutputs[0])
	}
	prompt, _ := firstChunk["prompt"].(string)
	if !strings.Contains(prompt, "以下是需要提炼的会议片段") {
		t.Fatalf("expected chunk prompt in detail payload, got %q", prompt)
	}
}

func TestMeetingSummaryHandlerUsesCustomChunkPrompt(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Messages) == 0 {
			t.Fatal("expected at least one message")
		}
		prompts = append(prompts, payload.Messages[0].Content)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"阶段摘要"}}],"model":"qwen3-4b","usage":{"prompt_tokens":100,"completion_tokens":50}}`))
	}))
	defer server.Close()

	handler := NewMeetingSummaryHandler(nil)
	inputText := strings.Repeat("这是一段需要分片处理的会议内容。", 400)
	configBytes := []byte(`{
		"endpoint":"` + server.URL + `",
		"model":"qwen3-4b",
		"chunk_prompt_template":"自定义分片模板：{{Text}}",
		"prompt_template":"最终模板：{{TEXT}}",
		"max_tokens":2048
	}`)
	if _, _, err := handler.Execute(context.Background(), configBytes, inputText, nil); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(prompts) < 2 {
		t.Fatalf("expected chunk and final requests, got %d", len(prompts))
	}
	if !strings.Contains(prompts[0], "自定义分片模板") || strings.Contains(prompts[0], "{{Text}}") {
		t.Fatalf("expected custom chunk prompt with replaced placeholder, got %q", prompts[0])
	}
}
