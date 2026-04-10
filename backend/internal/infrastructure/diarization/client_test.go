package diarization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewClientDoesNotUseHardcodedTimeout(t *testing.T) {
	t.Parallel()

	client := NewClient("http://example.com")
	if client.httpClient.Timeout != 0 {
		t.Fatalf("expected client timeout to be unset, got %s", client.httpClient.Timeout)
	}
}

func TestAnalyzeFileDecodesWrappedSegmentsResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/diarize" {
			t.Fatalf("expected request path /diarize, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"task_id":      "abc123",
			"num_speakers": 1,
			"segments": []map[string]any{{
				"speaker":  "spk_0",
				"start":    0.5,
				"end":      1.75,
				"duration": 1.25,
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "sample.wav")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0o600); err != nil {
		t.Fatalf("write temp audio: %v", err)
	}

	client := NewClient(server.URL)
	segments, err := client.AnalyzeFile(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Speaker != "spk_0" {
		t.Fatalf("expected speaker spk_0, got %q", segments[0].Speaker)
	}
	if segments[0].StartTime != 0.5 {
		t.Fatalf("expected start_time 0.5, got %v", segments[0].StartTime)
	}
	if segments[0].EndTime != 1.75 {
		t.Fatalf("expected end_time 1.75, got %v", segments[0].EndTime)
	}
}

func TestAnalyzeDecodesLegacySegmentsArray(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/diarize" {
			t.Fatalf("expected request path /diarize, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected application/json content type, got %q", got)
		}
		if err := json.NewEncoder(w).Encode([]map[string]any{{
			"speaker":    "speaker_1",
			"start_time": 2.5,
			"end_time":   4.0,
		}}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	segments, err := client.Analyze(context.Background(), "http://example.com/audio.wav")
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Speaker != "speaker_1" {
		t.Fatalf("expected speaker speaker_1, got %q", segments[0].Speaker)
	}
	if segments[0].StartTime != 2.5 {
		t.Fatalf("expected start_time 2.5, got %v", segments[0].StartTime)
	}
	if segments[0].EndTime != 4.0 {
		t.Fatalf("expected end_time 4.0, got %v", segments[0].EndTime)
	}
}
