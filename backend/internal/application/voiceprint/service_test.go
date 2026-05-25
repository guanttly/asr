package voiceprint

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lgt/asr/internal/infrastructure/diarization"
)

type voiceprintClientStub struct {
	metadata diarization.VoiceprintMetadata
}

func (s *voiceprintClientStub) BaseURL() string {
	return "http://speaker-analysis:8100"
}

func (s *voiceprintClientStub) ListVoiceprints(context.Context) ([]diarization.VoiceprintRecord, error) {
	return nil, nil
}

func (s *voiceprintClientStub) EnrollVoiceprint(_ context.Context, _ string, metadata diarization.VoiceprintMetadata) (*diarization.VoiceprintRecord, error) {
	s.metadata = metadata
	return &diarization.VoiceprintRecord{SpeakerName: metadata.SpeakerName}, nil
}

func (s *voiceprintClientStub) DeleteVoiceprint(context.Context, string) error {
	return nil
}

func TestEnrollValidatesSpeakerNameLength(t *testing.T) {
	service := NewService(&voiceprintClientStub{})

	_, err := service.Enroll(context.Background(), &EnrollRequest{
		SpeakerName:   strings.Repeat("张", 65),
		AudioFilePath: "/tmp/sample.wav",
	})
	if !errors.Is(err, ErrSpeakerNameTooLong) {
		t.Fatalf("expected ErrSpeakerNameTooLong, got %v", err)
	}
}

func TestEnrollTrimsSpeakerNameAtLimit(t *testing.T) {
	client := &voiceprintClientStub{}
	service := NewService(client)

	name := strings.Repeat("李", 64)
	_, err := service.Enroll(context.Background(), &EnrollRequest{
		SpeakerName:   " " + name + " ",
		AudioFilePath: "/tmp/sample.wav",
	})
	if err != nil {
		t.Fatalf("Enroll returned error: %v", err)
	}
	if client.metadata.SpeakerName != name {
		t.Fatalf("expected trimmed speaker name at limit, got %q", client.metadata.SpeakerName)
	}
}
