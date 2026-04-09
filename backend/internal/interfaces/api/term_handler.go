package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appterm "github.com/lgt/asr/internal/application/terminology"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// TermHandler exposes terminology management endpoints.
type TermHandler struct {
	service *appterm.Service
}

// NewTermHandler creates a terminology handler.
func NewTermHandler(service *appterm.Service) *TermHandler {
	return &TermHandler{service: service}
}

// Register registers terminology routes.
func (h *TermHandler) Register(group *gin.RouterGroup) {
	group.GET("/term-dicts", h.ListDicts)
	group.POST("/term-dicts", h.CreateDict)
	group.PUT("/term-dicts/:id", h.UpdateDict)
	group.DELETE("/term-dicts/:id", h.DeleteDict)
	group.GET("/term-dicts/:id/entries", h.ListEntries)
	group.POST("/term-dicts/:id/entries", h.CreateEntry)
	group.PUT("/term-dicts/:id/entries/:entryId", h.UpdateEntry)
	group.DELETE("/term-dicts/:id/entries/:entryId", h.DeleteEntry)
	group.GET("/term-dicts/:id/rules", h.ListRules)
	group.POST("/term-dicts/:id/rules", h.CreateRule)
	group.PUT("/term-dicts/:id/rules/:ruleId", h.UpdateRule)
	group.DELETE("/term-dicts/:id/rules/:ruleId", h.DeleteRule)
	group.POST("/term-dicts/:id/import", h.BatchImport)
}

func (h *TermHandler) ListDicts(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	items, total, err := h.service.ListDicts(c.Request.Context(), offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"items": items, "total": total})
}

func (h *TermHandler) CreateDict(c *gin.Context) {
	var req appterm.CreateDictRequest
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

func (h *TermHandler) UpdateDict(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	var req appterm.UpdateDictRequest
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

func (h *TermHandler) DeleteDict(c *gin.Context) {
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

func (h *TermHandler) ListEntries(c *gin.Context) {
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

func (h *TermHandler) CreateEntry(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	var req appterm.CreateEntryRequest
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

func (h *TermHandler) UpdateEntry(c *gin.Context) {
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

	var req appterm.UpdateEntryRequest
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

func (h *TermHandler) DeleteEntry(c *gin.Context) {
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

func (h *TermHandler) ListRules(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	items, err := h.service.GetDictRules(c.Request.Context(), dictID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, items)
}

func (h *TermHandler) CreateRule(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	var req appterm.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.DictID = dictID

	result, err := h.service.CreateRule(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) UpdateRule(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("ruleId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid rule id")
		return
	}

	var req appterm.UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.ID = ruleID
	req.DictID = dictID

	result, err := h.service.UpdateRule(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) DeleteRule(c *gin.Context) {
	ruleID, err := strconv.ParseUint(c.Param("ruleId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid rule id")
		return
	}

	if err := h.service.DeleteRule(c.Request.Context(), ruleID); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"deleted": true})
}

func (h *TermHandler) BatchImport(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	var req appterm.BatchImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.DictID = dictID

	if err := h.service.BatchImport(c.Request.Context(), &req); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"imported": len(req.Entries)})
}
