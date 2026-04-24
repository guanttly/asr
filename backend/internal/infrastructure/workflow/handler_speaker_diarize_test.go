package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lgt/asr/internal/infrastructure/diarization"
)

func TestSpeakerDiarizeHandlerFallsBackOnServiceError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"service unavailable"}`, http.StatusBadGateway)
	}))
	defer server.Close()

	handler := NewSpeakerDiarizeHandler(diarization.NewClient(server.URL), nil)
	output, detail, err := handler.Execute(context.Background(), json.RawMessage(`{"fail_on_error":false}`), "原始文本", &ExecutionMeta{AudioURL: "http://example.com/audio.wav"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if output != "原始文本" {
		t.Fatalf("expected passthrough text, got %q", output)
	}
	if !strings.Contains(string(detail), "diarization skipped") {
		t.Fatalf("expected warning detail, got %s", string(detail))
	}
}

func TestSpeakerDiarizeHandlerUsesVoiceprintIdentifyEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/diarize-identify" {
			t.Fatalf("expected identify endpoint, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"segments": []map[string]any{{
				"speaker_id": "张三",
				"start_time": 0.0,
				"end_time":   2.0,
				"confidence": 0.93,
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "meeting.wav")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0o600); err != nil {
		t.Fatalf("write temp audio: %v", err)
	}

	handler := NewSpeakerDiarizeHandler(nil, nil)
	output, detail, err := handler.Execute(context.Background(), json.RawMessage(`{"service_url":"`+server.URL+`","enable_voiceprint_match":true}`), "会议文本", &ExecutionMeta{AudioFilePath: audioPath})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(output, "张三") {
		t.Fatalf("expected output to contain matched speaker, got %q", output)
	}
	if !strings.Contains(string(detail), `"enable_voiceprint_match":true`) {
		t.Fatalf("expected detail to mark voiceprint match enabled, got %s", string(detail))
	}
}

func TestSpeakerDiarizeHandlerUsesVoiceprintDefaultClientWhenEnabled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/diarize-identify" {
			t.Fatalf("expected identify endpoint, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"segments": []map[string]any{{
				"speaker_id": "李四",
				"start_time": 1.0,
				"end_time":   3.0,
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "meeting.wav")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0o600); err != nil {
		t.Fatalf("write temp audio: %v", err)
	}

	handler := NewSpeakerDiarizeHandler(nil, diarization.NewClient(server.URL))
	output, detail, err := handler.Execute(context.Background(), json.RawMessage(`{"enable_voiceprint_match":true}`), "会议文本", &ExecutionMeta{AudioFilePath: audioPath})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(output, "李四") {
		t.Fatalf("expected output to contain matched speaker, got %q", output)
	}
	if !strings.Contains(string(detail), server.URL) {
		t.Fatalf("expected detail to contain selected service url, got %s", string(detail))
	}
}

func TestSpeakerDiarizeHandlerNormalizesAnonymousSpeakerLabels(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"segments": []map[string]any{
				{
					"speaker_id": "speaker_0",
					"start_time": 0.0,
					"end_time":   2.0,
				},
				{
					"speaker_id": "speaker_1",
					"start_time": 2.0,
					"end_time":   4.0,
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := NewSpeakerDiarizeHandler(diarization.NewClient(server.URL), nil)
	output, detail, err := handler.Execute(context.Background(), nil, "会议文本", &ExecutionMeta{AudioURL: "http://example.com/audio.wav"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(output, "speaker_0") || strings.Contains(output, "speaker_1") {
		t.Fatalf("expected output to use normalized labels, got %q", output)
	}
	if !strings.Contains(output, "说话人1") || !strings.Contains(output, "说话人2") {
		t.Fatalf("expected output to contain normalized labels, got %q", output)
	}

	var payload struct {
		Segments []struct {
			Speaker string `json:"speaker"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(detail, &payload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if len(payload.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(payload.Segments))
	}
	if payload.Segments[0].Speaker != "说话人1" || payload.Segments[1].Speaker != "说话人2" {
		t.Fatalf("expected normalized segment labels, got %+v", payload.Segments)
	}
}

func TestSpeakerDiarizeHandlerKeepsTranscriptTextPerSegment(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"segments": []map[string]any{
				{
					"speaker_id": "speaker_0",
					"start_time": 0.0,
					"end_time":   2.0,
				},
				{
					"speaker_id": "speaker_1",
					"start_time": 2.0,
					"end_time":   4.0,
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := NewSpeakerDiarizeHandler(diarization.NewClient(server.URL), nil)
	output, _, err := handler.Execute(context.Background(), nil, "第一句。第二句。", &ExecutionMeta{AudioURL: "http://example.com/audio.wav"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(output, "[说话人1 0.0s-2.0s] 第一句。") {
		t.Fatalf("expected first segment to keep transcript text, got %q", output)
	}
	if !strings.Contains(output, "[说话人2 2.0s-4.0s] 第二句。") {
		t.Fatalf("expected second segment to keep transcript text, got %q", output)
	}
	if strings.Contains(output, "\n\n第一句。第二句。") {
		t.Fatalf("expected transcript to be merged into segment lines, got %q", output)
	}
}

func TestSpeakerDiarizeHandlerDropsEmptyTranscriptSegments(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"segments": []map[string]any{
				{"speaker_id": "speaker_0", "start_time": 0.0, "end_time": 1.0},
				{"speaker_id": "speaker_1", "start_time": 1.0, "end_time": 2.0},
				{"speaker_id": "speaker_2", "start_time": 2.0, "end_time": 3.0},
				{"speaker_id": "speaker_3", "start_time": 3.0, "end_time": 4.0},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	handler := NewSpeakerDiarizeHandler(diarization.NewClient(server.URL), nil)
	output, detail, err := handler.Execute(context.Background(), nil, "甲。乙。", &ExecutionMeta{AudioURL: "http://example.com/audio.wav"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(output, "[说话人2 1.0s-2.0s]\n") || strings.Contains(output, "[说话人4 3.0s-4.0s]\n") {
		t.Fatalf("expected empty speaker lines to be removed, got %q", output)
	}

	var payload struct {
		Segments []struct {
			Speaker string `json:"speaker"`
		} `json:"segments"`
		SegmentsCount int `json:"segments_count"`
	}
	if err := json.Unmarshal(detail, &payload); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if payload.SegmentsCount != len(payload.Segments) {
		t.Fatalf("expected segments_count to match filtered segments, got count=%d segments=%d", payload.SegmentsCount, len(payload.Segments))
	}
	if payload.SegmentsCount >= 4 {
		t.Fatalf("expected empty transcript segments to be filtered, got %+v", payload)
	}
}
