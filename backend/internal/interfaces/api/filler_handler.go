package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appfiller "github.com/lgt/asr/internal/application/filler"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// FillerHandler exposes filler dictionary management endpoints.
type FillerHandler struct {
	service *appfiller.Service
}

func NewFillerHandler(service *appfiller.Service) *FillerHandler {
	return &FillerHandler{service: service}
}

func (h *FillerHandler) Register(group *gin.RouterGroup) {
	group.GET("/filler-dicts", h.ListDicts)
	group.POST("/filler-dicts", h.CreateDict)
	group.PUT("/filler-dicts/:id", h.UpdateDict)
	group.DELETE("/filler-dicts/:id", h.DeleteDict)
	group.GET("/filler-dicts/:id/entries", h.ListEntries)
	group.POST("/filler-dicts/:id/entries", h.CreateEntry)
	group.PUT("/filler-dicts/:id/entries/:entryId", h.UpdateEntry)
	group.DELETE("/filler-dicts/:id/entries/:entryId", h.DeleteEntry)
}

func (h *FillerHandler) ListDicts(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, total, err := h.service.ListDicts(c.Request.Context(), offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{"items": items, "total": total})
}

func (h *FillerHandler) CreateDict(c *gin.Context) {
	var req appfiller.CreateDictRequest
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

func (h *FillerHandler) UpdateDict(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	var req appfiller.UpdateDictRequest
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

func (h *FillerHandler) DeleteDict(c *gin.Context) {
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

func (h *FillerHandler) ListEntries(c *gin.Context) {
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

func (h *FillerHandler) CreateEntry(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	var req appfiller.CreateEntryRequest
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

func (h *FillerHandler) UpdateEntry(c *gin.Context) {
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
	var req appfiller.UpdateEntryRequest
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

func (h *FillerHandler) DeleteEntry(c *gin.Context) {
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

func (h *FillerHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appfiller.ErrFillerDictNotFound), errors.Is(err, appfiller.ErrFillerEntryNotFound):
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
	case errors.Is(err, appfiller.ErrFillerBaseDictProtected), errors.Is(err, appfiller.ErrFillerBaseDictConflict):
		response.Error(c, http.StatusConflict, errcode.CodeBadRequest, err.Error())
	default:
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
	}
}
