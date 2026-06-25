package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config is the shared runtime configuration for all backend apps.
type Config struct {
	AppName      string             `mapstructure:"app_name"`
	Log          LogConfig          `mapstructure:"log"`
	Server       ServerConfig       `mapstructure:"server"`
	Database     DatabaseConfig     `mapstructure:"database"`
	JWT          JWTConfig          `mapstructure:"jwt"`
	OpenAuth     OpenAuthConfig     `mapstructure:"open_auth"`
	Bootstrap    BootstrapConfig    `mapstructure:"bootstrap"`
	Product      ProductConfig      `mapstructure:"product"`
	Services     ServiceConfig      `mapstructure:"services"`
	Upload       UploadConfig       `mapstructure:"upload"`
	Meeting      MeetingConfig      `mapstructure:"meeting"`
	Cleanup      CleanupConfig      `mapstructure:"cleanup"`
	Download     DownloadConfig     `mapstructure:"download"`
	Catalog      CatalogConfig      `mapstructure:"catalog"`
	RulesCatalog RulesCatalogConfig `mapstructure:"rules_catalog"`
	Gateway      GatewayConfig      `mapstructure:"gateway"`
	Legacy       LegacyConfig       `mapstructure:"legacy"`
}

// LogConfig controls structured logging behaviour shared by all apps.
type LogConfig struct {
	// Level is the minimum level to emit: debug, info, warn, error.
	Level string `mapstructure:"level"`
}

// ServerConfig describes HTTP server settings.
type ServerConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	ASRAPIPort   int    `mapstructure:"asr_api_port"`
	AdminAPIPort int    `mapstructure:"admin_api_port"`
	NLPAPIPort   int    `mapstructure:"nlp_api_port"`
}

// DatabaseConfig contains MySQL connection info.
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

// DSN returns the MySQL DSN string.
func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.DBName,
	)
}

// JWTConfig holds JWT signing configuration.
type JWTConfig struct {
	Secret    string `mapstructure:"secret"`
	ExpiresIn int64  `mapstructure:"expires_in"`
}

type OpenAuthConfig struct {
	PlatformSecret   string `mapstructure:"platform_secret"`
	TokenExpiresIn   int64  `mapstructure:"token_expires_in"`
	LogRetentionDays int    `mapstructure:"log_retention_days"`
	BodyLogDir       string `mapstructure:"body_log_dir"`
}

// BootstrapConfig holds bootstrap account settings.
type BootstrapConfig struct {
	AdminUsername    string `mapstructure:"admin_username"`
	AdminPassword    string `mapstructure:"admin_password"`
	AdminDisplayName string `mapstructure:"admin_display_name"`
}

type ProductEdition string

const (
	ProductEditionStandard ProductEdition = "standard"
	ProductEditionAdvanced ProductEdition = "advanced"
)

type ProductFeatures struct {
	Edition              ProductEdition             `json:"edition"`
	Realtime             bool                       `json:"realtime"`
	Batch                bool                       `json:"batch"`
	Meeting              bool                       `json:"meeting"`
	Voiceprint           bool                       `json:"voiceprint"`
	VoiceControl         bool                       `json:"voice_control"`
	SupportedLanguages   []ProductLanguage          `json:"supported_languages"`
	HardwareTier         ProductEdition             `json:"hardware_tier"`
	HardwareRequirements map[string]HardwareProfile `json:"hardware_requirements"`
}

type ProductLanguage struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type HardwareSpec struct {
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	Storage      string `json:"storage"`
	Acceleration string `json:"acceleration"`
}

type HardwareProfile struct {
	Tier        ProductEdition `json:"tier"`
	Minimum     HardwareSpec   `json:"minimum"`
	Recommended HardwareSpec   `json:"recommended"`
}

type ProductConfig struct {
	Edition ProductEdition `mapstructure:"edition"`
}

func (c ProductConfig) NormalizedEdition() ProductEdition {
	switch ProductEdition(strings.ToLower(strings.TrimSpace(string(c.Edition)))) {
	case ProductEditionAdvanced:
		return ProductEditionAdvanced
	default:
		return ProductEditionStandard
	}
}

func (c ProductConfig) Features() ProductFeatures {
	edition := c.NormalizedEdition()
	features := ProductFeatures{
		Edition:              edition,
		Realtime:             true,
		Batch:                true,
		Meeting:              false,
		Voiceprint:           false,
		VoiceControl:         false,
		SupportedLanguages:   defaultProductLanguages(),
		HardwareTier:         edition,
		HardwareRequirements: defaultHardwareProfiles(),
	}
	if features.Edition == ProductEditionAdvanced {
		features.Meeting = true
		features.Voiceprint = true
		features.VoiceControl = true
	}
	return features
}

func defaultProductLanguages() []ProductLanguage {
	// 首项必须是 auto：医学场景常有中英混合（缩写/单位/药名），
	// 锁中文会让英文术语被错听，锁英文会丢中文病历主体。
	return []ProductLanguage{
		{Code: "auto", Label: "自动识别（中英混合）"},
		{Code: "zh-CN", Label: "普通话"},
		{Code: "en-US", Label: "英文（美）"},
	}
}

func defaultHardwareProfiles() map[string]HardwareProfile {
	return map[string]HardwareProfile{
		string(ProductEditionStandard): {
			Tier: ProductEditionStandard,
			Minimum: HardwareSpec{
				CPU:          "8 核",
				Memory:       "16 GB",
				Storage:      "200 GB SSD",
				Acceleration: "RTX 3090",
			},
			Recommended: HardwareSpec{
				CPU:          "16 核",
				Memory:       "32 GB",
				Storage:      "500 GB SSD",
				Acceleration: "A10 / A100",
			},
		},
		string(ProductEditionAdvanced): {
			Tier: ProductEditionAdvanced,
			Minimum: HardwareSpec{
				CPU:          "16 核",
				Memory:       "32 GB",
				Storage:      "500 GB SSD",
				Acceleration: "A10",
			},
			Recommended: HardwareSpec{
				CPU:          "16 核及以上",
				Memory:       "32 GB 及以上",
				Storage:      "500 GB SSD 及以上",
				Acceleration: "A100",
			},
		},
	}
}

// ServiceConfig holds downstream service endpoints.
type ServiceConfig struct {
	ASR                         string `mapstructure:"asr"`
	ASRMaxAudioSizeMB           int64  `mapstructure:"asr_max_audio_size_mb"`
	ASRStream                   string `mapstructure:"asr_stream"`
	ASRStreamSessionRolloverSec int    `mapstructure:"asr_stream_session_rollover_sec"`
	ASRBatchSyncIntervalSec     int    `mapstructure:"asr_batch_sync_interval_sec"`
	ASRBatchSyncBatchSize       int    `mapstructure:"asr_batch_sync_batch_size"`
	ASRBatchSyncWarnThreshold   int    `mapstructure:"asr_batch_sync_warn_threshold"`
	DashboardRetryHistoryLimit  int    `mapstructure:"dashboard_retry_history_limit"`
	SpeakerServiceURL           string `mapstructure:"speaker_service_url"`
	SummaryModel                string `mapstructure:"summary_model"`
}

// UploadConfig holds local upload storage settings.
type UploadConfig struct {
	Dir            string `mapstructure:"dir"`
	PublicBaseURL  string `mapstructure:"public_base_url"`
	MaxAudioSizeMB int64  `mapstructure:"max_audio_size_mb"`
	// MaxChunkSizeMB caps a single chunk body in the chunked upload protocol.
	MaxChunkSizeMB int64 `mapstructure:"max_chunk_size_mb"`
	// MaxSessionSizeMB caps the total assembled size of a chunked upload.
	MaxSessionSizeMB int64 `mapstructure:"max_session_size_mb"`
}

// MeetingConfig tunes the durable, resumable meeting upload pipeline that keeps
// long recordings safe by streaming them to the server progressively instead of
// buffering the whole recording in the client's memory.
type MeetingConfig struct {
	// MinRecordingDurationSec is the cumulative duration at which an in-progress
	// recording is promoted to a real (resumable) meeting. Recordings shorter
	// than this never become meetings.
	MinRecordingDurationSec float64 `mapstructure:"min_recording_duration_sec"`
	// UploadSessionInactiveTimeoutMinutes is how long a recording session may go
	// without a heartbeat before it is marked interrupted and recovered
	// server-side.
	UploadSessionInactiveTimeoutMinutes int `mapstructure:"upload_session_inactive_timeout_minutes"`
	// UploadSessionResumeRetentionDays is how long an interrupted session's temp
	// segments are kept so a client can resume (or the server can recover) it.
	UploadSessionResumeRetentionDays int `mapstructure:"upload_session_resume_retention_days"`
	// CompletedTempRetentionHours is how long a completed session's temp PCM
	// segments are kept after the formal audio has been assembled.
	CompletedTempRetentionHours int `mapstructure:"completed_temp_retention_hours"`
	// FormalAudioRetentionDays is the retention policy for assembled meeting
	// audio. 0 disables automatic enforcement (the default; deleting user audio
	// is destructive and opt-in).
	FormalAudioRetentionDays int `mapstructure:"formal_audio_retention_days"`
}

// CleanupConfig drives the background reclamation task that removes orphaned
// upload temp data and stale logs without ever touching active recordings.
type CleanupConfig struct {
	// IntervalMinutes is how often the cleanup task runs.
	IntervalMinutes int `mapstructure:"interval_minutes"`
	// LogRetentionDays is the retention for general application logs. 0 disables.
	LogRetentionDays int `mapstructure:"log_retention_days"`
	// OpenapiBodyLogRetentionDays is the retention for OpenAPI request/response
	// body logs. 0 falls back to open_auth.log_retention_days.
	OpenapiBodyLogRetentionDays int `mapstructure:"openapi_body_log_retention_days"`
}

// DownloadConfig holds local downloadable package storage settings.
type DownloadConfig struct {
	Dir            string `mapstructure:"dir"`
	PublicBasePath string `mapstructure:"public_base_path"`
}

// CatalogConfig points at the directory the radiology term catalog markdown
// files live in. Empty means "use the snapshot compiled into the binary".
type CatalogConfig struct {
	Dir string `mapstructure:"dir"`
}

// RulesCatalogConfig points at the directory the radiology rules catalog markdown
// files live in. Empty means "use the snapshot compiled into the binary".
type RulesCatalogConfig struct {
	Dir string `mapstructure:"dir"`
}

// GatewayConfig holds upstream addresses for the gateway app.
type GatewayConfig struct {
	ASRAPI   string `mapstructure:"asr_api"`
	AdminAPI string `mapstructure:"admin_api"`
	NLPAPI   string `mapstructure:"nlp_api"`
}

type LegacyConfig struct {
	Enabled                     bool   `mapstructure:"enabled"`
	AccessLogPath               string `mapstructure:"access_log_path"`
	DefaultWorkflowIDForASR     uint64 `mapstructure:"default_workflow_id_for_asr"`
	DefaultWorkflowIDForMeeting uint64 `mapstructure:"default_workflow_id_for_meeting"`
}

// Load reads configuration from a YAML file and matching environment variables.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("ASR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("app_name", "asr")
	v.SetDefault("log.level", "info")
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 10010)
	v.SetDefault("server.asr_api_port", 10011)
	v.SetDefault("server.admin_api_port", 10012)
	v.SetDefault("server.nlp_api_port", 10013)
	v.SetDefault("database.host", "127.0.0.1")
	v.SetDefault("database.port", 3306)
	v.SetDefault("database.user", "root")
	v.SetDefault("database.password", "root")
	v.SetDefault("database.dbname", "asr")
	v.SetDefault("jwt.secret", "dev-secret")
	v.SetDefault("jwt.expires_in", 86400)
	v.SetDefault("open_auth.platform_secret", "open-platform-dev-secret")
	v.SetDefault("open_auth.token_expires_in", 7200)
	v.SetDefault("open_auth.log_retention_days", 30)
	v.SetDefault("open_auth.body_log_dir", "runtime/openapi-logs")
	v.SetDefault("bootstrap.admin_username", "admin")
	v.SetDefault("bootstrap.admin_password", "123456")
	v.SetDefault("bootstrap.admin_display_name", "系统管理员")
	v.SetDefault("product.edition", string(ProductEditionStandard))
	v.SetDefault("services.asr", "http://127.0.0.1:9000")
	v.SetDefault("services.asr_max_audio_size_mb", 25)
	v.SetDefault("services.asr_stream", "")
	v.SetDefault("services.asr_stream_session_rollover_sec", 900)
	v.SetDefault("services.asr_batch_sync_interval_sec", 20)
	v.SetDefault("services.asr_batch_sync_batch_size", 20)
	v.SetDefault("services.asr_batch_sync_warn_threshold", 3)
	v.SetDefault("services.dashboard_retry_history_limit", 5)
	v.SetDefault("services.speaker_service_url", "http://127.0.0.1:9002")
	v.SetDefault("services.summary_model", "qwen3-4b")
	v.SetDefault("upload.dir", "uploads")
	v.SetDefault("upload.public_base_url", "")
	v.SetDefault("upload.max_audio_size_mb", 200)
	v.SetDefault("upload.max_chunk_size_mb", 8)
	v.SetDefault("upload.max_session_size_mb", 4096)
	v.SetDefault("meeting.min_recording_duration_sec", 5)
	v.SetDefault("meeting.upload_session_inactive_timeout_minutes", 60)
	v.SetDefault("meeting.upload_session_resume_retention_days", 7)
	v.SetDefault("meeting.completed_temp_retention_hours", 24)
	v.SetDefault("meeting.formal_audio_retention_days", 180)
	v.SetDefault("cleanup.interval_minutes", 10)
	v.SetDefault("cleanup.log_retention_days", 30)
	v.SetDefault("cleanup.openapi_body_log_retention_days", 30)
	v.SetDefault("download.dir", "downloads")
	v.SetDefault("download.public_base_path", "/downloads/files")
	v.SetDefault("catalog.dir", "")
	v.SetDefault("rules_catalog.dir", "")
	v.SetDefault("legacy.enabled", true)
	v.SetDefault("legacy.access_log_path", "runtime/legacy-access.log")
	v.SetDefault("legacy.default_workflow_id_for_asr", 0)
	v.SetDefault("legacy.default_workflow_id_for_meeting", 0)

	_ = v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Gateway upstreams default to the locally-configured service ports so that
	// changing server.*_port does not leave the gateway pointing at a stale
	// hard-coded port. Explicit values (e.g. cross-container hosts) win.
	if strings.TrimSpace(cfg.Gateway.ASRAPI) == "" {
		cfg.Gateway.ASRAPI = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.ASRAPIPort)
	}
	if strings.TrimSpace(cfg.Gateway.AdminAPI) == "" {
		cfg.Gateway.AdminAPI = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.AdminAPIPort)
	}
	if strings.TrimSpace(cfg.Gateway.NLPAPI) == "" {
		cfg.Gateway.NLPAPI = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.NLPAPIPort)
	}

	return &cfg, nil
}
