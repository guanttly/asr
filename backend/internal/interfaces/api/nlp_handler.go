package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	appnlp "github.com/lgt/asr/internal/application/nlp"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// NLPHandler exposes correction and summary endpoints.
type NLPHandler struct {
	service *appnlp.Service
}

// NewNLPHandler creates a new NLP handler.
func NewNLPHandler(service *appnlp.Service) *NLPHandler {
	return &NLPHandler{service: service}
}

// Register registers nlp routes.
func (h *NLPHandler) Register(group *gin.RouterGroup) {
	group.POST("/correct", h.Correct)
	group.POST("/summarize", h.Summarize)
}

func (h *NLPHandler) Correct(c *gin.Context) {
	var req appnlp.CorrectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.Correct(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *NLPHandler) Summarize(c *gin.Context) {
	var req appnlp.SummarizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.Summarize(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}
