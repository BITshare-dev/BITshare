package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/model"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type AdminAuthHandler struct {
	authService    *service.AdminAuthService
	sessionManager *session.Manager
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminProfileResponse struct {
	ID          string                  `json:"id"`
	Username    string                  `json:"username"`
	Role        string                  `json:"role"`
	Status      model.AdminStatus       `json:"status"`
	Permissions []model.AdminPermission `json:"permissions"`
}

type loginResponse struct {
	Admin adminProfileResponse `json:"admin"`
}

func NewAdminAuthHandler(authService *service.AdminAuthService, sessionManager *session.Manager) *AdminAuthHandler {
	return &AdminAuthHandler{
		authService:    authService,
		sessionManager: sessionManager,
	}
}

func (h *AdminAuthHandler) Login(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	authenticated, err := h.authService.Login(ctx.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidAdminCredentials) {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid username or password",
			})
			return
		}

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "login failed",
		})
		return
	}

	h.sessionManager.WriteCookie(ctx.Writer, authenticated.Cookie, authenticated.Identity.ExpiresAt)
	ctx.JSON(http.StatusOK, loginResponse{
		Admin: toAdminProfileResponse(authenticated.Admin),
	})
}

func (h *AdminAuthHandler) Logout(ctx *gin.Context) {
	cookieValue, err := ctx.Cookie(h.sessionManager.CookieName())
	if err == nil {
		if destroyErr := h.authService.Logout(ctx.Request.Context(), cookieValue); destroyErr != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "logout failed",
			})
			return
		}
	}

	h.sessionManager.ClearCookie(ctx.Writer)
	ctx.Status(http.StatusNoContent)
}

func (h *AdminAuthHandler) Me(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	ctx.JSON(http.StatusOK, loginResponse{
		Admin: adminProfileResponse{
			ID:          identity.AdminID,
			Username:    identity.Username,
			Role:        identity.Role,
			Status:      identity.Status,
			Permissions: identity.Permissions,
		},
	})
}

func (h *AdminAuthHandler) PermissionProbe(permission model.AdminPermission) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		identity, _ := session.GetAdminIdentity(ctx)
		ctx.JSON(http.StatusOK, gin.H{
			"admin_id":    identity.AdminID,
			"permission":  permission,
			"authorized":  true,
			"super_admin": identity.IsSuperAdmin(),
		})
	}
}

func toAdminProfileResponse(admin *model.Admin) adminProfileResponse {
	return adminProfileResponse{
		ID:          admin.ID,
		Username:    strings.TrimSpace(admin.Username),
		Role:        admin.Role,
		Status:      admin.Status,
		Permissions: admin.PermissionList(),
	}
}
