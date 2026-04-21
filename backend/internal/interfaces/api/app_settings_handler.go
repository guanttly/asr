package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lgt/asr/internal/application/appsettings"
	userdomain "github.com/lgt/asr/internal/domain/user"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// AppSettingsHandler exposes app-wide configuration endpoints, currently focused
// on the desktop terminal voice control settings.
type AppSettingsHandler struct {
	service *appsettings.Service
}

// NewAppSettingsHandler creates an app settings handler.
func NewAppSettingsHandler(service *appsettings.Service) *AppSettingsHandler {
	return &AppSettingsHandler{service: service}
}

// Register registers the app settings routes onto the protected admin group.
func (h *AppSettingsHandler) Register(group *gin.RouterGroup) {
	group.GET("/app-settings/voice-control", h.GetVoiceControl)
	group.PUT("/app-settings/voice-control", h.UpdateVoiceControl)
}

// GetVoiceControl returns the current voice control configuration. Available to
// any authenticated user (including anonymous desktop terminals).
func (h *AppSettingsHandler) GetVoiceControl(c *gin.Context) {
	cfg, err := h.service.GetVoiceControl(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, cfg)
}

// UpdateVoiceControl updates the voice control runtime parameters. Admin only.
func (h *AppSettingsHandler) UpdateVoiceControl(c *gin.Context) {
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
