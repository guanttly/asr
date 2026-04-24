package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	appvoiceprint "github.com/lgt/asr/internal/application/voiceprint"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// VoiceprintHandler exposes speaker voiceprint management endpoints.
type VoiceprintHandler struct {
	service        *appvoiceprint.Service
	maxAudioSizeMB int64
	feature        featureGate
}

// NewVoiceprintHandler creates a new voiceprint handler.
func NewVoiceprintHandler(service *appvoiceprint.Service, maxAudioSizeMB int64, features pkgconfig.ProductFeatures) *VoiceprintHandler {
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 100
	}
	return &VoiceprintHandler{service: service, maxAudioSizeMB: maxAudioSizeMB, feature: newFeatureGate(features)}
}

// Register registers voiceprint routes.
func (h *VoiceprintHandler) Register(group *gin.RouterGroup) {
	group.GET("", h.List)
	group.POST("", h.Enroll)
	group.DELETE("/:id", h.Delete)
}

func (h *VoiceprintHandler) List(c *gin.Context) {
	if !h.feature.voiceprint() {
		h.feature.denyFeature(c, "当前版本未开放声纹库")
		return
	}
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}

	response.Success(c, gin.H{
		"items":       items,
		"total":       len(items),
		"service_url": h.service.BaseURL(),
	})
}

func (h *VoiceprintHandler) Enroll(c *gin.Context) {
	if !h.feature.voiceprint() {
		h.feature.denyFeature(c, "当前版本未开放声纹库")
		return
	}
	audioFile, err := saveTemporaryUploadedAudio(c, "file", "voiceprint", h.maxAudioSizeMB)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		response.Error(c, status, errcode.CodeBadRequest, messageText)
		return
	}
	defer os.Remove(audioFile.AbsolutePath)

	record, err := h.service.Enroll(c.Request.Context(), &appvoiceprint.EnrollRequest{
		SpeakerName:   c.PostForm("speaker_name"),
		Department:    c.PostForm("department"),
		Notes:         c.PostForm("notes"),
		AudioFilePath: audioFile.AbsolutePath,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}

	response.Success(c, gin.H{
		"record":      record,
		"service_url": h.service.BaseURL(),
	})
}

func (h *VoiceprintHandler) Delete(c *gin.Context) {
	if !h.feature.voiceprint() {
		h.feature.denyFeature(c, "当前版本未开放声纹库")
		return
	}
	recordID := strings.TrimSpace(c.Param("id"))
	if err := h.service.Delete(c.Request.Context(), recordID); err != nil {
		h.writeError(c, err)
		return
	}

	response.Success(c, gin.H{"deleted": true, "id": recordID})
}

func (h *VoiceprintHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appvoiceprint.ErrServiceUnavailable):
		response.Error(c, http.StatusServiceUnavailable, errcode.CodeInternal, err.Error())
		return
	case errors.Is(err, appvoiceprint.ErrMissingSpeakerName), errors.Is(err, appvoiceprint.ErrMissingAudioFile), errors.Is(err, appvoiceprint.ErrMissingRecordID):
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	if statusCode := appvoiceprint.HTTPStatusCode(err); statusCode != 0 {
		response.Error(c, statusCode, errCodeForStatus(statusCode), err.Error())
		return
	}

	response.Error(c, http.StatusBadGateway, errcode.CodeInternal, err.Error())
}

func errCodeForStatus(statusCode int) int {
	switch statusCode {
	case http.StatusBadRequest:
		return errcode.CodeBadRequest
	case http.StatusUnauthorized:
		return errcode.CodeUnauthorized
	case http.StatusForbidden:
		return errcode.CodeForbidden
	case http.StatusNotFound:
		return errcode.CodeNotFound
	default:
		return errcode.CodeInternal
	}
}
