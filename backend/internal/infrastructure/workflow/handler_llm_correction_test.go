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

func TestNormalizeOpenAIChatEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{
			name:  "base url",
			input: "http://127.0.0.1:8000",
			want:  "http://127.0.0.1:8000/v1/chat/completions",
		},
		{
			name:  "base url with v1 suffix",
			input: "http://127.0.0.1:8000/v1",
			want:  "http://127.0.0.1:8000/v1/chat/completions",
		},
		{
			name:  "full chat completions url",
			input: "http://127.0.0.1:8000/v1/chat/completions",
			want:  "http://127.0.0.1:8000/v1/chat/completions",
		},
		{
			name:  "dashscope compatible mode base url",
			input: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			want:  "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		},
		{
			name:  "dashscope compatible mode root",
			input: "https://dashscope.aliyuncs.com/compatible-mode",
			want:  "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeOpenAIChatEndpoint(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("normalizeOpenAIChatEndpoint(%q) error = %v, want substring %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeOpenAIChatEndpoint(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeOpenAIChatEndpoint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInferAllowedCompletionTokens(t *testing.T) {
	body := `{"error":{"message":"max_tokens or 'max_completion_tokens' is too large: 4096. This model's maximum context length is 8192 tokens and your request has 6336 input tokens (4096 > 8192 - 6336)."}}`
	allowed, ok := inferAllowedCompletionTokens(body, 4096)
	if !ok {
		t.Fatal("expected parser to infer allowed tokens")
	}
	if allowed != 1856 {
		t.Fatalf("expected allowed tokens 1856, got %d", allowed)
	}
}

func TestLLMCorrectionHandlerRetriesWithReducedMaxTokensOnContextOverflow(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNumber := callCount.Add(1)
		var payload struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if callNumber == 1 {
			if payload.MaxTokens != 4096 {
				t.Fatalf("expected first request max_tokens=4096, got %d", payload.MaxTokens)
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"max_tokens or 'max_completion_tokens' is too large: 4096. This model's maximum context length is 8192 tokens and your request has 6336 input tokens (4096 > 8192 - 6336). None","type":"BadRequestError","code":400}}`))
			return
		}

		if payload.MaxTokens != 1856 {
			t.Fatalf("expected retried request max_tokens=1856, got %d", payload.MaxTokens)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"整理后的文本"}}],"model":"qwen3-4b","usage":{"prompt_tokens":6336,"completion_tokens":512}}`))
	}))
	defer server.Close()

	handler := NewLLMCorrectionHandler()
	configBytes := []byte(`{
		"endpoint": "` + server.URL + `",
		"model": "qwen3-4b",
		"prompt_template": "请整理以下文本：\n{{TEXT}}",
		"temperature": 0.3,
		"max_tokens": 4096
	}`)

	output, detail, err := handler.Execute(context.Background(), configBytes, strings.Repeat("会议内容", 100), nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if output != "整理后的文本" {
		t.Fatalf("expected corrected text, got %q", output)
	}
	if callCount.Load() != 2 {
		t.Fatalf("expected 2 requests, got %d", callCount.Load())
	}
	var parsedDetail map[string]any
	if err := json.Unmarshal(detail, &parsedDetail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if parsedDetail["max_tokens"] != float64(1856) {
		t.Fatalf("expected detail max_tokens=1856, got %+v", parsedDetail["max_tokens"])
	}
}

func TestLLMCorrectionHandlerNormalizesMarkdownOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"## 核心内容\n- 第一条\n- 第二条\n\n1. 第三条"}}],"model":"qwen3-4b","usage":{"prompt_tokens":200,"completion_tokens":80}}`))
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

	output, detail, err := handler.Execute(context.Background(), configBytes, "原始文本", nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if strings.Contains(output, "##") || strings.Contains(output, "- ") || strings.Contains(output, "1. ") {
		t.Fatalf("expected normalized plain text output, got %q", output)
	}
	if !strings.Contains(output, "核心内容") || !strings.Contains(output, "第一条") {
		t.Fatalf("expected normalized content to keep text body, got %q", output)
	}

	var parsedDetail map[string]any
	if err := json.Unmarshal(detail, &parsedDetail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if parsedDetail["normalized_markdown"] != true {
		t.Fatalf("expected normalized_markdown=true, got %+v", parsedDetail["normalized_markdown"])
	}
}
