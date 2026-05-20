package asrengine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSubmitBatchFallsBackToOpenAIOnTranscribeTransportError(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/transcribe":
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("response writer does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("hijack response: %v", err)
			}
			_ = conn.Close()
		case "/v1/audio/transcriptions":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart form: %v", err)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("expected multipart file: %v", err)
			}
			_ = file.Close()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"text":"fallback ok","usage":{"seconds":1.25}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 25)
	result, err := client.SubmitBatch(context.Background(), BatchTranscribeRequest{LocalFilePath: audioPath})
	if err != nil {
		t.Fatalf("SubmitBatch returned error: %v", err)
	}
	if result.ResultText != "fallback ok" || result.Duration != 1.25 || result.Status != "completed" {
		t.Fatalf("unexpected fallback result: %+v", result)
	}
}
