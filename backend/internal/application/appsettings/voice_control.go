package appsettings

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	VoiceControlKey         = "voice_control"
	DefaultCommandTimeoutMs = 10000
)

type VoiceControlConfig struct {
	CommandTimeoutMs int  `json:"command_timeout_ms"`
	Enabled          bool `json:"enabled"`
}

type SettingsRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

type Service struct {
	repo SettingsRepository
}

func NewService(repo SettingsRepository) *Service {
	return &Service{repo: repo}
}

func defaultVoiceControl() VoiceControlConfig {
	return VoiceControlConfig{
		CommandTimeoutMs: DefaultCommandTimeoutMs,
		Enabled:          true,
	}
}

func normalizeVoiceControl(cfg *VoiceControlConfig) VoiceControlConfig {
	out := defaultVoiceControl()
	if cfg == nil {
		return out
	}
	if cfg.CommandTimeoutMs > 0 {
		out.CommandTimeoutMs = cfg.CommandTimeoutMs
	}
	out.Enabled = cfg.Enabled
	return out
}

func (s *Service) GetVoiceControl(ctx context.Context) (VoiceControlConfig, error) {
	if s == nil || s.repo == nil {
		return defaultVoiceControl(), nil
	}
	raw, err := s.repo.Get(ctx, VoiceControlKey)
	if err != nil {
		return VoiceControlConfig{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return defaultVoiceControl(), nil
	}
	var cfg VoiceControlConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return VoiceControlConfig{}, fmt.Errorf("invalid stored voice_control config: %w", err)
	}
	return normalizeVoiceControl(&cfg), nil
}

func (s *Service) UpdateVoiceControl(ctx context.Context, cfg *VoiceControlConfig) (VoiceControlConfig, error) {
	if s == nil || s.repo == nil {
		return VoiceControlConfig{}, fmt.Errorf("app settings repository not configured")
	}
	normalized := normalizeVoiceControl(cfg)
	data, err := json.Marshal(&normalized)
	if err != nil {
		return VoiceControlConfig{}, err
	}
	if err := s.repo.Set(ctx, VoiceControlKey, string(data)); err != nil {
		return VoiceControlConfig{}, err
	}
	return normalized, nil
}
