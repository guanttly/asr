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
	ServiceURL string `json:"service_url"`
}

// SpeakerDiarizeHandler calls an external diarization service and merges
// speaker labels into the text. This node requires audio context from ExecutionMeta.
type SpeakerDiarizeHandler struct {
	defaultClient *diarization.Client
}

func NewSpeakerDiarizeHandler(defaultClient *diarization.Client) *SpeakerDiarizeHandler {
	return &SpeakerDiarizeHandler{defaultClient: defaultClient}
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
	if cfg.ServiceURL != "" {
		client = diarization.NewClient(cfg.ServiceURL)
	}
	if client == nil {
		return inputText, nil, fmt.Errorf("no diarization service configured")
	}

	var segments []diarization.Segment
	var err error
	if meta.AudioFilePath != "" {
		segments, err = client.AnalyzeFile(ctx, meta.AudioFilePath)
	} else {
		segments, err = client.Analyze(ctx, meta.AudioURL)
	}
	if err != nil {
		return inputText, nil, fmt.Errorf("diarization failed: %w", err)
	}

	// Format segments with speaker labels
	var lines []string
	for _, seg := range segments {
		lines = append(lines, fmt.Sprintf("[%s %.1f-%.1f] %s", seg.Speaker, seg.StartTime, seg.EndTime, ""))
	}

	// If we have segments, prepend speaker info to text
	if len(segments) > 0 {
		var sb strings.Builder
		for _, seg := range segments {
			sb.WriteString(fmt.Sprintf("[%s %.1fs-%.1fs]\n", seg.Speaker, seg.StartTime, seg.EndTime))
		}
		sb.WriteString("\n")
		sb.WriteString(inputText)
		inputText = sb.String()
	}

	detail, _ := json.Marshal(map[string]interface{}{
		"segments_count": len(segments),
		"segments":       segments,
	})
	return inputText, detail, nil
}
