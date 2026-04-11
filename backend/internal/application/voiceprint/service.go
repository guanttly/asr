package voiceprint

import (
	"context"
	"errors"
	"strings"

	"github.com/lgt/asr/internal/infrastructure/diarization"
)

var (
	ErrServiceUnavailable = errors.New("未配置 Speaker Analysis Service 地址，请设置 services.speaker_analysis_url")
	ErrMissingSpeakerName = errors.New("speaker_name is required")
	ErrMissingAudioFile   = errors.New("voiceprint audio file is required")
	ErrMissingRecordID    = errors.New("voiceprint id is required")
)

type client interface {
	BaseURL() string
	ListVoiceprints(ctx context.Context) ([]diarization.VoiceprintRecord, error)
	EnrollVoiceprint(ctx context.Context, audioFilePath string, metadata diarization.VoiceprintMetadata) (*diarization.VoiceprintRecord, error)
	DeleteVoiceprint(ctx context.Context, recordID string) error
}

type httpStatusCoder interface {
	HTTPStatusCode() int
}

// Service manages registered speaker voiceprints.
type Service struct {
	client client
}

// NewService creates a voiceprint service.
func NewService(client client) *Service {
	return &Service{client: client}
}

// BaseURL returns the configured upstream base URL.
func (s *Service) BaseURL() string {
	if s == nil || s.client == nil {
		return ""
	}
	return strings.TrimSpace(s.client.BaseURL())
}

// HTTPStatusCode extracts the upstream status code from an error when available.
func HTTPStatusCode(err error) int {
	var statusErr httpStatusCoder
	if errors.As(err, &statusErr) {
		return statusErr.HTTPStatusCode()
	}
	return 0
}

// List returns all registered voiceprints.
func (s *Service) List(ctx context.Context) ([]Record, error) {
	if err := s.ensureAvailable(); err != nil {
		return nil, err
	}

	records, err := s.client.ListVoiceprints(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]Record, 0, len(records))
	for _, item := range records {
		items = append(items, mapRecord(item))
	}
	return items, nil
}

// Enroll registers a new voiceprint using a local audio file.
func (s *Service) Enroll(ctx context.Context, req *EnrollRequest) (*Record, error) {
	if err := s.ensureAvailable(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrMissingAudioFile
	}

	speakerName := strings.TrimSpace(req.SpeakerName)
	if speakerName == "" {
		return nil, ErrMissingSpeakerName
	}
	audioFilePath := strings.TrimSpace(req.AudioFilePath)
	if audioFilePath == "" {
		return nil, ErrMissingAudioFile
	}

	record, err := s.client.EnrollVoiceprint(ctx, audioFilePath, diarization.VoiceprintMetadata{
		SpeakerName: speakerName,
		Department:  strings.TrimSpace(req.Department),
		Notes:       strings.TrimSpace(req.Notes),
	})
	if err != nil {
		return nil, err
	}

	result := mapRecord(*record)
	return &result, nil
}

// Delete removes a registered voiceprint.
func (s *Service) Delete(ctx context.Context, recordID string) error {
	if err := s.ensureAvailable(); err != nil {
		return err
	}
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return ErrMissingRecordID
	}
	return s.client.DeleteVoiceprint(ctx, recordID)
}

func (s *Service) ensureAvailable() error {
	if strings.TrimSpace(s.BaseURL()) == "" {
		return ErrServiceUnavailable
	}
	return nil
}

func mapRecord(record diarization.VoiceprintRecord) Record {
	return Record{
		ID:            record.ID,
		SpeakerName:   record.SpeakerName,
		Department:    record.Department,
		Notes:         record.Notes,
		AudioDuration: record.AudioDuration,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}
