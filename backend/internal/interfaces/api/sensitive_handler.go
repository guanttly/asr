package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appsensitive "github.com/lgt/asr/internal/application/sensitive"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// SensitiveHandler exposes sensitive dictionary management endpoints.
type SensitiveHandler struct {
	service *appsensitive.Service
}

func NewSensitiveHandler(service *appsensitive.Service) *SensitiveHandler {
	return &SensitiveHandler{service: service}
}

func (h *SensitiveHandler) Register(group *gin.RouterGroup) {
	group.GET("/sensitive-dicts", h.ListDicts)
	group.POST("/sensitive-dicts", h.CreateDict)
	group.PUT("/sensitive-dicts/:id", h.UpdateDict)
	group.DELETE("/sensitive-dicts/:id", h.DeleteDict)
	group.GET("/sensitive-dicts/:id/entries", h.ListEntries)
	group.POST("/sensitive-dicts/:id/entries", h.CreateEntry)
	group.PUT("/sensitive-dicts/:id/entries/:entryId", h.UpdateEntry)
	group.DELETE("/sensitive-dicts/:id/entries/:entryId", h.DeleteEntry)
}

func (h *SensitiveHandler) ListDicts(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, total, err := h.service.ListDicts(c.Request.Context(), offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{"items": items, "total": total})
}

func (h *SensitiveHandler) CreateDict(c *gin.Context) {
	var req appsensitive.CreateDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.CreateDict(c.Request.Context(), &req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SensitiveHandler) UpdateDict(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	var req appsensitive.UpdateDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.UpdateDict(c.Request.Context(), dictID, &req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SensitiveHandler) DeleteDict(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	if err := h.service.DeleteDict(c.Request.Context(), dictID); err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *SensitiveHandler) ListEntries(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	items, err := h.service.GetDictEntries(c.Request.Context(), dictID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *SensitiveHandler) CreateEntry(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	var req appsensitive.CreateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.DictID = dictID
	result, err := h.service.CreateEntry(c.Request.Context(), &req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SensitiveHandler) UpdateEntry(c *gin.Context) {
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
	var req appsensitive.UpdateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.ID = entryID
	req.DictID = dictID
	result, err := h.service.UpdateEntry(c.Request.Context(), &req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SensitiveHandler) DeleteEntry(c *gin.Context) {
	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid entry id")
		return
	}
	if err := h.service.DeleteEntry(c.Request.Context(), entryID); err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *SensitiveHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appsensitive.ErrSensitiveDictNotFound), errors.Is(err, appsensitive.ErrSensitiveEntryNotFound):
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
	case errors.Is(err, appsensitive.ErrSensitiveBaseDictProtected), errors.Is(err, appsensitive.ErrSensitiveBaseDictConflict):
		response.Error(c, http.StatusConflict, errcode.CodeBadRequest, err.Error())
	default:
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
	}
}
