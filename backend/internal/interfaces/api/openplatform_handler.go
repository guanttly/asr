package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appopenplatform "github.com/lgt/asr/internal/application/openplatform"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
	userdomain "github.com/lgt/asr/internal/domain/user"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

type OpenPlatformHandler struct {
	service *appopenplatform.Service
}

func NewOpenPlatformHandler(service *appopenplatform.Service) *OpenPlatformHandler {
	return &OpenPlatformHandler{service: service}
}

func (h *OpenPlatformHandler) RegisterAdmin(group *gin.RouterGroup) {
	open := group.Group("/openplatform")
	open.Use(h.adminOnly())
	open.GET("/capabilities", h.ListCapabilities)
	open.GET("/apps", h.ListApps)
	open.POST("/apps", h.CreateApp)
	open.GET("/apps/:id", h.GetApp)
	open.PUT("/apps/:id", h.UpdateApp)
	open.DELETE("/apps/:id", h.RevokeApp)
	open.POST("/apps/:id/rotate-secret", h.RotateSecret)
	open.POST("/apps/:id/disable", h.DisableApp)
	open.POST("/apps/:id/enable", h.EnableApp)
	open.GET("/apps/:id/calls", h.ListAppCalls)
}

func (h *OpenPlatformHandler) RegisterOpenAuth(group *gin.RouterGroup) {
	group.POST("/token", h.IssueToken)
}

func (h *OpenPlatformHandler) ListCapabilities(c *gin.Context) {
	response.Success(c, gin.H{"items": h.service.ListCapabilities()})
}

func (h *OpenPlatformHandler) ListApps(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, total, err := h.service.ListApps(c.Request.Context(), offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{"items": items, "total": total})
}

func (h *OpenPlatformHandler) CreateApp(c *gin.Context) {
	var req appopenplatform.CreateAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.CreateApp(c.Request.Context(), middleware.UserIDFromContext(c), &req)
	if err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpenPlatformHandler) GetApp(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	result, err := h.service.GetApp(c.Request.Context(), id)
	if err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpenPlatformHandler) UpdateApp(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req appopenplatform.UpdateAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	result, err := h.service.UpdateApp(c.Request.Context(), id, &req)
	if err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpenPlatformHandler) RevokeApp(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.RevokeApp(c.Request.Context(), id); err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, gin.H{"revoked": true})
}

func (h *OpenPlatformHandler) RotateSecret(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	result, err := h.service.RotateSecret(c.Request.Context(), id)
	if err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpenPlatformHandler) DisableApp(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DisableApp(c.Request.Context(), id); err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, gin.H{"status": openplatformdomain.AppStatusDisabled})
}

func (h *OpenPlatformHandler) EnableApp(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.EnableApp(c.Request.Context(), id); err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, gin.H{"status": openplatformdomain.AppStatusActive})
}

func (h *OpenPlatformHandler) ListAppCalls(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	items, err := h.service.ListAppCalls(c.Request.Context(), id, limit)
	if err != nil {
		h.writeAdminError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *OpenPlatformHandler) IssueToken(c *gin.Context) {
	var req appopenplatform.IssueTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
		return
	}
	result, err := h.service.IssueToken(c.Request.Context(), &req)
	if err != nil {
		h.writeOpenAuthError(c, err)
		return
	}
	response.OpenSuccess(c, result)
}

func (h *OpenPlatformHandler) adminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if middleware.RoleFromContext(c) != string(userdomain.RoleAdmin) {
			response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "admin role required")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *OpenPlatformHandler) writeAdminError(c *gin.Context, err error) {
	var validationErr *appopenplatform.ValidationError
	switch {
	case errors.Is(err, openplatformdomain.ErrAppNotFound):
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
	case errors.Is(err, openplatformdomain.ErrAppAlreadyExists):
		response.Error(c, http.StatusConflict, errcode.CodeBadRequest, err.Error())
	case errors.Is(err, appopenplatform.ErrCallbackWhitelistReq):
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
	case errors.As(err, &validationErr):
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, validationErr.Error())
	default:
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
	}
}

func (h *OpenPlatformHandler) writeOpenAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appopenplatform.ErrOpenAuthInvalid):
		response.OpenError(c, http.StatusUnauthorized, errcode.OpenAuthInvalid, "invalid app_id or app_secret")
	case errors.Is(err, appopenplatform.ErrOpenAppDisabled):
		response.OpenError(c, http.StatusForbidden, errcode.OpenAppDisabled, "application is disabled")
	default:
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
	}
}

func parseUintParam(c *gin.Context, key string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}
