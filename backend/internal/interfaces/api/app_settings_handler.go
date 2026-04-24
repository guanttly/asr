package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lgt/asr/internal/application/appsettings"
	userdomain "github.com/lgt/asr/internal/domain/user"
	"github.com/lgt/asr/internal/interfaces/middleware"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// AppSettingsHandler exposes app-wide configuration endpoints, currently focused
// on the desktop terminal voice control settings.
type AppSettingsHandler struct {
	service *appsettings.Service
	feature featureGate
}

// NewAppSettingsHandler creates an app settings handler.
func NewAppSettingsHandler(service *appsettings.Service, features pkgconfig.ProductFeatures) *AppSettingsHandler {
	return &AppSettingsHandler{service: service, feature: newFeatureGate(features)}
}

// Register registers the app settings routes onto the protected admin group.
func (h *AppSettingsHandler) Register(group *gin.RouterGroup) {
	group.GET("/app-settings/product-features", h.GetProductFeatures)
	group.GET("/app-settings/voice-control", h.GetVoiceControl)
	group.PUT("/app-settings/voice-control", h.UpdateVoiceControl)
}

// GetProductFeatures returns the current product edition and capability flags.
func (h *AppSettingsHandler) GetProductFeatures(c *gin.Context) {
	response.Success(c, h.feature.payload())
}

// GetVoiceControl returns the current voice control configuration. Available to
// any authenticated user (including anonymous desktop terminals).
func (h *AppSettingsHandler) GetVoiceControl(c *gin.Context) {
	if !h.feature.voiceControl() {
		response.Success(c, appsettings.VoiceControlConfig{CommandTimeoutMs: appsettings.DefaultCommandTimeoutMs, Enabled: false})
		return
	}
	cfg, err := h.service.GetVoiceControl(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, cfg)
}

// UpdateVoiceControl updates the voice control runtime parameters. Admin only.
func (h *AppSettingsHandler) UpdateVoiceControl(c *gin.Context) {
	if !h.feature.voiceControl() {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "当前版本未开放终端语音控制")
		return
	}
	if middleware.RoleFromContext(c) != string(userdomain.RoleAdmin) {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "管理员才能修改语音控制配置")
		return
	}
	var req appsettings.VoiceControlConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	cfg, err := h.service.UpdateVoiceControl(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, cfg)
}
