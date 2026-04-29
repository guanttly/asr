package openplatform

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
)

type CapabilityDescriptor struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

type CreateAppRequest struct {
	Name              string            `json:"name" binding:"required"`
	Description       string            `json:"description"`
	AllowedCaps       []string          `json:"allowed_caps" binding:"required"`
	DefaultWorkflows  map[string]uint64 `json:"default_workflows"`
	CallbackWhitelist []string          `json:"callback_whitelist"`
	RateLimitPerSec   uint32            `json:"rate_limit_per_sec"`
	MetaJSON          string            `json:"meta_json"`
}

type UpdateAppRequest struct {
	Name              string            `json:"name" binding:"required"`
	Description       string            `json:"description"`
	AllowedCaps       []string          `json:"allowed_caps" binding:"required"`
	DefaultWorkflows  map[string]uint64 `json:"default_workflows"`
	CallbackWhitelist []string          `json:"callback_whitelist"`
	RateLimitPerSec   uint32            `json:"rate_limit_per_sec"`
	MetaJSON          string            `json:"meta_json"`
}

type AppResponse struct {
	ID                uint64                       `json:"id"`
	AppID             string                       `json:"app_id"`
	Name              string                       `json:"name"`
	Description       string                       `json:"description"`
	SecretHint        string                       `json:"secret_hint,omitempty"`
	SecretVersion     uint32                       `json:"secret_version"`
	Status            openplatformdomain.AppStatus `json:"status"`
	RateLimitPerSec   uint32                       `json:"rate_limit_per_sec"`
	AllowedCaps       []string                     `json:"allowed_caps"`
	DefaultWorkflows  map[string]uint64            `json:"default_workflows,omitempty"`
	CallbackWhitelist []string                     `json:"callback_whitelist,omitempty"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
}

type CreateAppResponse struct {
	AppResponse
	AppSecret string `json:"app_secret"`
}

type IssueTokenRequest struct {
	AppID     string `json:"app_id" binding:"required"`
	AppSecret string `json:"app_secret" binding:"required"`
}

type IssueTokenResponse struct {
	AccessToken string   `json:"access_token"`
	ExpiresIn   int64    `json:"expires_in"`
	TokenType   string   `json:"token_type"`
	AllowedCaps []string `json:"allowed_caps"`
}

type CreateSkillRequest struct {
	Name              string   `json:"name" binding:"required"`
	DisplayName       string   `json:"display_name" binding:"required"`
	Description       string   `json:"description"`
	IntentPatterns    []string `json:"intent_patterns" binding:"required"`
	ParametersSchema  string   `json:"parameters"`
	CallbackURL       string   `json:"callback_url" binding:"required"`
	CallbackTimeoutMs uint32   `json:"callback_timeout_ms"`
	Enabled           *bool    `json:"enabled"`
}

type UpdateSkillRequest struct {
	Name              string   `json:"name" binding:"required"`
	DisplayName       string   `json:"display_name" binding:"required"`
	Description       string   `json:"description"`
	IntentPatterns    []string `json:"intent_patterns" binding:"required"`
	ParametersSchema  string   `json:"parameters"`
	CallbackURL       string   `json:"callback_url" binding:"required"`
	CallbackTimeoutMs uint32   `json:"callback_timeout_ms"`
	Enabled           *bool    `json:"enabled"`
}

type SkillResponse struct {
	SkillID             string     `json:"skill_id"`
	Name                string     `json:"name"`
	DisplayName         string     `json:"display_name"`
	Description         string     `json:"description"`
	IntentPatterns      []string   `json:"intent_patterns"`
	ParametersSchema    string     `json:"parameters,omitempty"`
	CallbackURL         string     `json:"callback_url"`
	CallbackTimeoutMs   uint32     `json:"callback_timeout_ms"`
	Enabled             bool       `json:"enabled"`
	ConsecutiveFailures uint32     `json:"consecutive_failures"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type SkillDryRunRequest struct {
	Utterance string `json:"utterance" binding:"required"`
}

type SkillDryRunResponse struct {
	Matched             bool           `json:"matched"`
	MatchedPattern      string         `json:"matched_pattern,omitempty"`
	ExtractedParameters map[string]any `json:"extracted_parameters,omitempty"`
	WouldCallback       string         `json:"would_callback,omitempty"`
}

type AccessTokenClaims struct {
	AppID         string   `json:"app_id"`
	AllowedCaps   []string `json:"allowed_caps"`
	SecretVersion uint32   `json:"secret_version"`
	jwt.RegisteredClaims
}
