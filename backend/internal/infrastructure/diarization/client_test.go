package diarization

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestAnalyzeFileWithOptionsUsesIdentifyEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/diarize-identify" {
			t.Fatalf("expected request path /api/v1/diarize-identify, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"segments": []map[string]any{{
				"speaker_id": "张三",
				"start_time": 0.0,
				"end_time":   3.5,
				"confidence": 0.92,
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
	segments, err := client.AnalyzeFileWithOptions(context.Background(), audioPath, AnalyzeOptions{EnableVoiceprintMatch: true})
	if err != nil {
		t.Fatalf("AnalyzeFileWithOptions returned error: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}
	if segments[0].Speaker != "张三" {
		t.Fatalf("expected speaker 张三, got %q", segments[0].Speaker)
	}
}

func TestListVoiceprintsUsesAPIv1Endpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/voiceprint/list" {
			t.Fatalf("expected request path /api/v1/voiceprint/list, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"total": 1,
			"records": []map[string]any{{
				"id":             "vp-1",
				"speaker_name":   "张三",
				"department":     "产品部",
				"notes":          "测试样本",
				"audio_duration": 18.5,
				"created_at":     "2025-04-10T08:30:00",
				"updated_at":     "2025-04-10T08:30:00",
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	records, err := client.ListVoiceprints(context.Background())
	if err != nil {
		t.Fatalf("ListVoiceprints returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].ID != "vp-1" {
		t.Fatalf("expected id vp-1, got %q", records[0].ID)
	}
	if records[0].SpeakerName != "张三" {
		t.Fatalf("expected speaker_name 张三, got %q", records[0].SpeakerName)
	}
	if records[0].AudioDuration != 18.5 {
		t.Fatalf("expected audio_duration 18.5, got %v", records[0].AudioDuration)
	}
}

func TestEnrollVoiceprintUploadsFileAndMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/voiceprint/enroll" {
			t.Fatalf("expected request path /api/v1/voiceprint/enroll, got %s", r.URL.Path)
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse content type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("expected multipart/form-data, got %q", mediaType)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("speaker_name"); got != "李四" {
			t.Fatalf("expected speaker_name 李四, got %q", got)
		}
		if got := r.FormValue("department"); got != "研发部" {
			t.Fatalf("expected department 研发部, got %q", got)
		}
		if got := r.FormValue("notes"); got != "主讲人样本" {
			t.Fatalf("expected notes 主讲人样本, got %q", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}
		defer file.Close()
		if !strings.HasSuffix(header.Filename, ".wav") {
			t.Fatalf("expected uploaded wav file, got %q", header.Filename)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":             "vp-2",
			"speaker_name":   "李四",
			"department":     "研发部",
			"notes":          "主讲人样本",
			"audio_duration": 22.4,
			"created_at":     "2025-04-10T09:00:00",
			"updated_at":     "2025-04-10T09:00:00",
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
	record, err := client.EnrollVoiceprint(context.Background(), audioPath, VoiceprintMetadata{
		SpeakerName: "李四",
		Department:  "研发部",
		Notes:       "主讲人样本",
	})
	if err != nil {
		t.Fatalf("EnrollVoiceprint returned error: %v", err)
	}
	if record.ID != "vp-2" {
		t.Fatalf("expected id vp-2, got %q", record.ID)
	}
	if record.SpeakerName != "李四" {
		t.Fatalf("expected speaker_name 李四, got %q", record.SpeakerName)
	}
}
