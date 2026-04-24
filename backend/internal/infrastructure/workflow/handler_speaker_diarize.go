package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lgt/asr/internal/infrastructure/diarization"
)

// SpeakerDiarizeConfig is the configuration for the speaker diarization node.
type SpeakerDiarizeConfig struct {
	ServiceURL            string `json:"service_url"`
	EnableVoiceprintMatch bool   `json:"enable_voiceprint_match"`
	FailOnError           bool   `json:"fail_on_error"`
}

// SpeakerDiarizeHandler calls an external diarization service and merges
// speaker labels into the text. This node requires audio context from ExecutionMeta.
type SpeakerDiarizeHandler struct {
	defaultClient           *diarization.Client
	voiceprintDefaultClient *diarization.Client
}

func NewSpeakerDiarizeHandler(defaultClient, voiceprintDefaultClient *diarization.Client) *SpeakerDiarizeHandler {
	return &SpeakerDiarizeHandler{
		defaultClient:           defaultClient,
		voiceprintDefaultClient: voiceprintDefaultClient,
	}
}

func (h *SpeakerDiarizeHandler) Validate(config json.RawMessage) error {
	if len(config) == 0 || string(config) == "null" || string(config) == "{}" {
		if h.defaultClient == nil {
			return fmt.Errorf("service_url is required when no default diarization client is configured")
		}
		return nil
	}
	var cfg SpeakerDiarizeConfig
	return json.Unmarshal(config, &cfg)
}

func (h *SpeakerDiarizeHandler) Execute(ctx context.Context, config json.RawMessage, inputText string, meta *ExecutionMeta) (string, json.RawMessage, error) {
	if meta == nil || (meta.AudioURL == "" && meta.AudioFilePath == "") {
		// Cannot perform diarization without audio context; pass through text
		detail, _ := json.Marshal(map[string]string{"warning": "no audio context in execution context, skipping diarization"})
		return inputText, detail, nil
	}

	var cfg SpeakerDiarizeConfig
	if len(config) > 0 && string(config) != "null" {
		_ = json.Unmarshal(config, &cfg)
	}

	client := h.defaultClient
	if cfg.EnableVoiceprintMatch && h.voiceprintDefaultClient != nil {
		client = h.voiceprintDefaultClient
	}
	if cfg.ServiceURL != "" {
		client = diarization.NewClient(cfg.ServiceURL)
	}
	effectiveServiceURL := ""
	if client != nil {
		effectiveServiceURL = strings.TrimSpace(client.BaseURL())
	}
	if client == nil {
		if cfg.FailOnError {
			return inputText, nil, fmt.Errorf("no diarization service configured")
		}
		detail, _ := json.Marshal(map[string]any{"warning": "no diarization service configured, skipping diarization"})
		return inputText, detail, nil
	}

	var segments []diarization.Segment
	var err error
	if meta.AudioFilePath != "" {
		segments, err = client.AnalyzeFileWithOptions(ctx, meta.AudioFilePath, diarization.AnalyzeOptions{
			EnableVoiceprintMatch: cfg.EnableVoiceprintMatch,
		})
	} else {
		if cfg.EnableVoiceprintMatch {
			detail, _ := json.Marshal(map[string]any{"warning": "voiceprint match requires local audio file, falling back to anonymous diarization"})
			_ = detail
		}
		segments, err = client.Analyze(ctx, meta.AudioURL)
	}
	if err != nil {
		if cfg.FailOnError {
			return inputText, nil, fmt.Errorf("diarization failed: %w", err)
		}
		detail, _ := json.Marshal(map[string]any{
			"warning":                 fmt.Sprintf("diarization skipped: %v", err),
			"error":                   err.Error(),
			"enable_voiceprint_match": cfg.EnableVoiceprintMatch,
		})
		return inputText, detail, nil
	}

	normalizeWorkflowSpeakerSegments(segments)
	if len(segments) > 0 {
		parts := diarization.SplitTranscriptByDurations(inputText, workflowSpeakerSegmentDurations(segments))
		if strings.TrimSpace(inputText) != "" {
			filteredSegments, filteredParts := filterSpeakerDiarizeSegments(segments, parts)
			if len(filteredSegments) > 0 {
				segments = filteredSegments
				parts = filteredParts
			}
		}
		inputText = buildSpeakerDiarizeOutput(segments, parts)
	}

	detail, _ := json.Marshal(map[string]interface{}{
		"segments_count":          len(segments),
		"segments":                segments,
		"enable_voiceprint_match": cfg.EnableVoiceprintMatch,
		"diarization_service_url": effectiveServiceURL,
	})
	return inputText, detail, nil
}

func normalizeWorkflowSpeakerSegments(segments []diarization.Segment) {
	labels := make([]string, 0, len(segments))
	for _, seg := range segments {
		if label := strings.TrimSpace(seg.Speaker); label != "" {
			labels = append(labels, label)
		}
	}
	zeroBased := diarization.AnonymousSpeakerLabelsUseZeroBased(labels)
	for index := range segments {
		segments[index].Speaker = diarization.NormalizeAnonymousSpeakerLabel(segments[index].Speaker, zeroBased)
	}
}

func buildSpeakerDiarizeOutput(segments []diarization.Segment, parts []string) string {
	if len(segments) == 0 {
		return ""
	}

	var sb strings.Builder
	for index, seg := range segments {
		if index > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("[%s %.1fs-%.1fs]", seg.Speaker, seg.StartTime, seg.EndTime))
		if index < len(parts) {
			if text := strings.TrimSpace(parts[index]); text != "" {
				sb.WriteString(" ")
				sb.WriteString(text)
			}
		}
	}
	return sb.String()
}

func filterSpeakerDiarizeSegments(segments []diarization.Segment, parts []string) ([]diarization.Segment, []string) {
	filteredSegments := make([]diarization.Segment, 0, len(segments))
	filteredParts := make([]string, 0, len(segments))
	for index, seg := range segments {
		if index >= len(parts) {
			continue
		}
		text := strings.TrimSpace(parts[index])
		if text == "" {
			continue
		}
		filteredSegments = append(filteredSegments, seg)
		filteredParts = append(filteredParts, text)
	}
	return filteredSegments, filteredParts
}

func workflowSpeakerSegmentDurations(segments []diarization.Segment) []float64 {
	durations := make([]float64, 0, len(segments))
	for _, seg := range segments {
		duration := seg.EndTime - seg.StartTime
		if duration < 0 {
			duration = 0
		}
		durations = append(durations, duration)
	}
	return durations
}
