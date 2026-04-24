package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appvoicecommand "github.com/lgt/asr/internal/application/voicecommand"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

type VoiceCommandHandler struct {
	service *appvoicecommand.Service
	feature featureGate
}

func NewVoiceCommandHandler(service *appvoicecommand.Service, features pkgconfig.ProductFeatures) *VoiceCommandHandler {
	return &VoiceCommandHandler{service: service, feature: newFeatureGate(features)}
}

func (h *VoiceCommandHandler) Register(group *gin.RouterGroup) {
	group.GET("/voice-command-dicts", h.ListDicts)
	group.POST("/voice-command-dicts", h.CreateDict)
	group.PUT("/voice-command-dicts/:id", h.UpdateDict)
	group.DELETE("/voice-command-dicts/:id", h.DeleteDict)
	group.GET("/voice-command-dicts/:id/entries", h.ListEntries)
	group.POST("/voice-command-dicts/:id/entries", h.CreateEntry)
	group.PUT("/voice-command-dicts/:id/entries/:entryId", h.UpdateEntry)
	group.DELETE("/voice-command-dicts/:id/entries/:entryId", h.DeleteEntry)
}

func (h *VoiceCommandHandler) ListDicts(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, total, err := h.service.ListDicts(c.Request.Context(), offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{"items": items, "total": total})
}

func (h *VoiceCommandHandler) CreateDict(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	var req appvoicecommand.CreateDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.CreateDict(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *VoiceCommandHandler) UpdateDict(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	var req appvoicecommand.UpdateDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.UpdateDict(c.Request.Context(), dictID, &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *VoiceCommandHandler) DeleteDict(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	if err := h.service.DeleteDict(c.Request.Context(), dictID); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *VoiceCommandHandler) ListEntries(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	items, err := h.service.GetDictEntries(c.Request.Context(), dictID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, items)
}

func (h *VoiceCommandHandler) CreateEntry(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	var req appvoicecommand.CreateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.DictID = dictID
	result, err := h.service.CreateEntry(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *VoiceCommandHandler) UpdateEntry(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid entry id")
		return
	}
	var req appvoicecommand.UpdateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.ID = entryID
	req.DictID = dictID
	result, err := h.service.UpdateEntry(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *VoiceCommandHandler) DeleteEntry(c *gin.Context) {
	if !h.feature.voiceControl() {
		h.feature.denyFeature(c, "当前版本未开放控制指令库")
		return
	}
	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid entry id")
		return
	}
	if err := h.service.DeleteEntry(c.Request.Context(), entryID); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
