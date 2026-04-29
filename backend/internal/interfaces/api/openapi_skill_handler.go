package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	appopenplatform "github.com/lgt/asr/internal/application/openplatform"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

type OpenAPISkillHandler struct {
	service *appopenplatform.Service
}

func NewOpenAPISkillHandler(service *appopenplatform.Service) *OpenAPISkillHandler {
	return &OpenAPISkillHandler{service: service}
}

func (h *OpenAPISkillHandler) Register(group *gin.RouterGroup) {
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
	group.POST("/:id/dry-run", h.DryRun)
}

func (h *OpenAPISkillHandler) Create(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	var req appopenplatform.CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
		return
	}
	result, err := h.service.CreateSkill(c.Request.Context(), app, &req)
	if err != nil {
		h.writeSkillError(c, err)
		return
	}
	response.OpenSuccess(c, withRequestID(c, result))
}

func (h *OpenAPISkillHandler) List(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	items, err := h.service.ListSkills(c.Request.Context(), app)
	if err != nil {
		h.writeSkillError(c, err)
		return
	}
	response.OpenSuccess(c, withRequestID(c, gin.H{"items": items}))
}

func (h *OpenAPISkillHandler) Get(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	result, err := h.service.GetSkill(c.Request.Context(), app, c.Param("id"))
	if err != nil {
		h.writeSkillError(c, err)
		return
	}
	response.OpenSuccess(c, withRequestID(c, result))
}

func (h *OpenAPISkillHandler) Update(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	var req appopenplatform.UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
		return
	}
	result, err := h.service.UpdateSkill(c.Request.Context(), app, c.Param("id"), &req)
	if err != nil {
		h.writeSkillError(c, err)
		return
	}
	response.OpenSuccess(c, withRequestID(c, result))
}

func (h *OpenAPISkillHandler) Delete(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	if err := h.service.DeleteSkill(c.Request.Context(), app, c.Param("id")); err != nil {
		h.writeSkillError(c, err)
		return
	}
	response.OpenSuccess(c, withRequestID(c, gin.H{"deleted": true}))
}

func (h *OpenAPISkillHandler) DryRun(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	var req appopenplatform.SkillDryRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
		return
	}
	result, err := h.service.DryRunSkill(c.Request.Context(), app, c.Param("id"), req.Utterance)
	if err != nil {
		h.writeSkillError(c, err)
		return
	}
	response.OpenSuccess(c, withRequestID(c, result))
}

func (h *OpenAPISkillHandler) writeSkillError(c *gin.Context, err error) {
	var validationErr *appopenplatform.ValidationError
	switch {
	case errors.Is(err, openplatformdomain.ErrSkillNotFound):
		response.OpenError(c, http.StatusNotFound, errcode.OpenSkillNotFound, err.Error())
	case errors.Is(err, openplatformdomain.ErrSkillNameDuplicated):
		response.OpenError(c, http.StatusConflict, errcode.OpenSkillNameDuplicated, err.Error())
	case errors.Is(err, appopenplatform.ErrSkillCallbackUnreachable):
		response.OpenError(c, http.StatusUnprocessableEntity, errcode.OpenSkillCallbackUnreachable, err.Error())
	case errors.Is(err, appopenplatform.ErrSkillCallbackNotWhitelisted):
		response.OpenError(c, http.StatusUnprocessableEntity, errcode.OpenSkillCallbackNotWhitelisted, err.Error())
	case errors.As(err, &validationErr):
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, validationErr.Error())
	default:
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
	}
}

func withRequestID(c *gin.Context, data any) gin.H {
	return gin.H{"request_id": middleware.RequestIDFromContext(c), "data": data}
}
