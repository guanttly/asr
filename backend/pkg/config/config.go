package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config is the shared runtime configuration for all backend apps.
type Config struct {
	AppName   string          `mapstructure:"app_name"`
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	OpenAuth  OpenAuthConfig  `mapstructure:"open_auth"`
	Bootstrap BootstrapConfig `mapstructure:"bootstrap"`
	Product   ProductConfig   `mapstructure:"product"`
	Services  ServiceConfig   `mapstructure:"services"`
	Upload    UploadConfig    `mapstructure:"upload"`
	Download  DownloadConfig  `mapstructure:"download"`
	Catalog   CatalogConfig   `mapstructure:"catalog"`
	Gateway   GatewayConfig   `mapstructure:"gateway"`
	Legacy    LegacyConfig    `mapstructure:"legacy"`
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
	v.SetDefault("upload.max_audio_size_mb", 1024)
	v.SetDefault("download.dir", "downloads")
	v.SetDefault("download.public_base_path", "/downloads/files")
	v.SetDefault("catalog.dir", "")
	v.SetDefault("gateway.asr_api", "http://127.0.0.1:10011")
	v.SetDefault("gateway.admin_api", "http://127.0.0.1:10012")
	v.SetDefault("gateway.nlp_api", "http://127.0.0.1:10013")
	v.SetDefault("legacy.enabled", true)
	v.SetDefault("legacy.access_log_path", "runtime/legacy-access.log")
	v.SetDefault("legacy.default_workflow_id_for_asr", 0)
	v.SetDefault("legacy.default_workflow_id_for_meeting", 0)

	_ = v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
