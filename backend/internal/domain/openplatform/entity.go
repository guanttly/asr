package openplatform

import (
	"errors"
	"time"
)

var (
	ErrAppNotFound         = errors.New("open app not found")
	ErrAppAlreadyExists    = errors.New("open app already exists")
	ErrSkillNotFound       = errors.New("open skill not found")
	ErrSkillNameDuplicated = errors.New("open skill name duplicated")
	ErrCallLogNotFound     = errors.New("open call log not found")
	ErrInvocationNotFound  = errors.New("skill invocation not found")
)

type AppStatus string

const (
	AppStatusActive   AppStatus = "active"
	AppStatusDisabled AppStatus = "disabled"
	AppStatusRevoked  AppStatus = "revoked"
)

type InvocationStatus string

const (
	AppOwnerOffset                 uint64           = 1 << 63
	InvocationStatusSuccess        InvocationStatus = "success"
	InvocationStatusFailed         InvocationStatus = "failed"
	InvocationStatusTimeout        InvocationStatus = "timeout"
	InvocationStatusSignedRejected InvocationStatus = "signed_rejected"
)

func OwnerUserIDForApp(appID uint64) uint64 {
	return AppOwnerOffset + appID
}

func AppIDFromOwnerUserID(ownerUserID uint64) (uint64, bool) {
	if ownerUserID < AppOwnerOffset {
		return 0, false
	}
	return ownerUserID - AppOwnerOffset, true
}

type App struct {
	ID                  uint64            `json:"id"`
	AppID               string            `json:"app_id"`
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	SecretHint          string            `json:"secret_hint,omitempty"`
	AppSecretHash       string            `json:"-"`
	AppSecretCiphertext string            `json:"-"`
	SecretVersion       uint32            `json:"secret_version"`
	Status              AppStatus         `json:"status"`
	OwnerUserID         uint64            `json:"owner_user_id"`
	RateLimitPerSec     uint32            `json:"rate_limit_per_sec"`
	CallbackWhitelist   []string          `json:"callback_whitelist,omitempty"`
	DefaultWorkflows    map[string]uint64 `json:"default_workflows,omitempty"`
	AllowedCaps         []string          `json:"allowed_caps"`
	MetaJSON            string            `json:"meta_json,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

type Skill struct {
	ID                  uint64     `json:"id"`
	SkillUID            string     `json:"skill_id"`
	AppID               uint64     `json:"app_id"`
	Name                string     `json:"name"`
	DisplayName         string     `json:"display_name"`
	Description         string     `json:"description"`
	IntentPatterns      []string   `json:"intent_patterns"`
	ParametersSchema    string     `json:"parameters_schema,omitempty"`
	CallbackURL         string     `json:"callback_url"`
	CallbackTimeoutMs   uint32     `json:"callback_timeout_ms"`
	Enabled             bool       `json:"enabled"`
	ConsecutiveFailures uint32     `json:"consecutive_failures"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type CallLog struct {
	ID         uint64    `json:"id"`
	RequestID  string    `json:"request_id"`
	AppID      uint64    `json:"app_id"`
	Capability string    `json:"capability"`
	Route      string    `json:"route"`
	HTTPStatus uint16    `json:"http_status"`
	ErrCode    string    `json:"err_code,omitempty"`
	LatencyMs  uint32    `json:"latency_ms"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	BodyRef    string    `json:"body_ref,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type SkillInvocation struct {
	ID             uint64           `json:"id"`
	SkillID        uint64           `json:"skill_id"`
	AppID          uint64           `json:"app_id"`
	RequestID      string           `json:"request_id"`
	MatchedPattern string           `json:"matched_pattern,omitempty"`
	Utterance      string           `json:"utterance,omitempty"`
	ParametersJSON string           `json:"parameters_json,omitempty"`
	Status         InvocationStatus `json:"status"`
	HTTPStatus     *uint16          `json:"http_status,omitempty"`
	LatencyMs      *uint32          `json:"latency_ms,omitempty"`
	ErrorMessage   string           `json:"error_message,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}

type SkillInvokeResult struct {
	SkillID        string           `json:"skill_id"`
	SkillName      string           `json:"skill_name"`
	MatchedPattern string           `json:"matched_pattern,omitempty"`
	Status         InvocationStatus `json:"status"`
	HTTPStatus     *uint16          `json:"http_status,omitempty"`
	ResponseJSON   string           `json:"response_json,omitempty"`
	ErrorMessage   string           `json:"error_message,omitempty"`
}
