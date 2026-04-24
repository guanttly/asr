package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appuser "github.com/lgt/asr/internal/application/user"
	domain "github.com/lgt/asr/internal/domain/user"
	"github.com/lgt/asr/internal/interfaces/middleware"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// UserHandler exposes user and auth endpoints.
type UserHandler struct {
	service   *appuser.Service
	jwtSecret string
	expiresIn int64
	feature   featureGate
}

// NewUserHandler creates a user handler.
func NewUserHandler(service *appuser.Service, jwtSecret string, expiresIn int64, features pkgconfig.ProductFeatures) *UserHandler {
	return &UserHandler{service: service, jwtSecret: jwtSecret, expiresIn: expiresIn, feature: newFeatureGate(features)}
}

// RegisterPublic registers public auth routes.
func (h *UserHandler) RegisterPublic(group *gin.RouterGroup) {
	group.POST("/login", h.Login)
	group.POST("/anonymous-login", h.AnonymousLogin)
}

// RegisterProtected registers protected user routes.
func (h *UserHandler) RegisterProtected(group *gin.RouterGroup) {
	group.POST("/users", h.CreateUser)
	group.GET("/users", h.ListUsers)
	group.GET("/users/:id", h.GetUser)
	group.GET("/me", h.GetCurrentUser)
	group.PUT("/me/profile", h.UpdateCurrentUserProfile)
	group.GET("/me/workflow-bindings", h.GetCurrentUserWorkflowBindings)
	group.PUT("/me/workflow-bindings", h.UpdateCurrentUserWorkflowBindings)
}

func (h *UserHandler) Login(c *gin.Context) {
	var req appuser.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	user, err := h.service.Authenticate(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, errcode.CodeUnauthorized, err.Error())
		return
	}

	token, err := middleware.GenerateToken(h.jwtSecret, h.expiresIn, user.ID, string(user.Role))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, appuser.LoginResponse{Token: token, ExpiresIn: h.expiresIn})
}

func (h *UserHandler) AnonymousLogin(c *gin.Context) {
	var req appuser.AnonymousLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	clientIP := c.ClientIP()
	if clientIP != "" {
		req.IPAddresses = append(req.IPAddresses, clientIP)
	}

	user, err := h.service.AuthenticateAnonymously(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, errcode.CodeUnauthorized, err.Error())
		return
	}

	token, err := middleware.GenerateToken(h.jwtSecret, h.expiresIn, user.ID, string(user.Role))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, appuser.LoginResponse{Token: token, ExpiresIn: h.expiresIn, User: appuser.ToUserResponse(user)})
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	role := middleware.RoleFromContext(c)
	if role != "" && role != string(domain.RoleAdmin) {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "admin role required")
		return
	}

	var req appuser.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.CreateUser(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			response.Error(c, http.StatusConflict, errcode.CodeBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, total, err := h.service.ListUsers(c.Request.Context(), offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"items": items, "total": total})
}

func (h *UserHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid user id")
		return
	}

	result, err := h.service.GetUser(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	result, err := h.service.GetUser(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *UserHandler) UpdateCurrentUserProfile(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	var req appuser.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.UpdateProfile(c.Request.Context(), userID, &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *UserHandler) GetCurrentUserWorkflowBindings(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	result, err := h.service.GetWorkflowBindings(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}

	response.Success(c, h.feature.sanitizeWorkflowBindings(result))
}

func (h *UserHandler) UpdateCurrentUserWorkflowBindings(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	var req appuser.UpdateWorkflowBindingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	if err := h.feature.constrainWorkflowBindingsRequest(&req); err != nil {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, err.Error())
		return
	}

	result, err := h.service.UpdateWorkflowBindings(c.Request.Context(), userID, &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, h.feature.sanitizeWorkflowBindings(result))
}
