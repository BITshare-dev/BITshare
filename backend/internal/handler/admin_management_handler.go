package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/model"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type AdminManagementHandler struct {
	service     *service.AdminManagementService
	authService *service.AdminAuthService
}

type createAdminRequest struct {
	Permissions []model.AdminPermission `json:"permissions"`
}

type updateAdminRequest struct {
	Status      model.AdminStatus       `json:"status"`
	Permissions []model.AdminPermission `json:"permissions"`
}

type resetAdminPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

type deleteAdminRequest struct {
	Password string `json:"password"`
}

func NewAdminManagementHandler(service *service.AdminManagementService, authService *service.AdminAuthService) *AdminManagementHandler {
	return &AdminManagementHandler{service: service, authService: authService}
}

func (h *AdminManagementHandler) ListAdmins(ctx *gin.Context) {
	items, err := h.service.ListAdmins(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list admins"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AdminManagementHandler) CreateAdmin(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req createAdminRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.service.CreateAdmin(ctx.Request.Context(), service.CreateAdminInput{
		Permissions: req.Permissions,
		OperatorID:  identity.AdminID,
		OperatorIP:  ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid admin input"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create admin"})
		}
		return
	}
	ctx.JSON(http.StatusCreated, item)
}

func (h *AdminManagementHandler) UpdateAdmin(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req updateAdminRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	item, err := h.service.UpdateAdmin(ctx.Request.Context(), ctx.Param("adminID"), service.UpdateAdminInput{
		Status:      req.Status,
		Permissions: req.Permissions,
		OperatorID:  identity.AdminID,
		OperatorIP:  ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid admin input"})
		case errors.Is(err, service.ErrAdminNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "admin not found"})
		case errors.Is(err, service.ErrAdminImmutableTarget):
			ctx.JSON(http.StatusConflict, gin.H{"error": "cannot modify this admin"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update admin"})
		}
		return
	}
	ctx.JSON(http.StatusOK, item)
}

func (h *AdminManagementHandler) ResetPassword(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req resetAdminPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.ResetPassword(ctx.Request.Context(), ctx.Param("adminID"), service.ResetAdminPasswordInput{
		NewPassword: req.NewPassword,
		OperatorID:  identity.AdminID,
		OperatorIP:  ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid password"})
		case errors.Is(err, service.ErrAdminNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "admin not found"})
		case errors.Is(err, service.ErrAdminImmutableTarget):
			ctx.JSON(http.StatusConflict, gin.H{"error": "cannot modify this admin"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset password"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *AdminManagementHandler) DeleteAdmin(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	if !identity.IsSuperAdmin() {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "super admin required"})
		return
	}

	var req deleteAdminRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.authService.VerifyPassword(ctx.Request.Context(), identity.AdminID, req.Password); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAdminCredentials):
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify password"})
		}
		return
	}

	err := h.service.DeleteAdmin(ctx.Request.Context(), ctx.Param("adminID"), identity.AdminID, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "admin not found"})
		case errors.Is(err, service.ErrAdminImmutableTarget):
			ctx.JSON(http.StatusConflict, gin.H{"error": "cannot delete this admin"})
		case errors.Is(err, service.ErrAdminDeleteDenied):
			ctx.JSON(http.StatusConflict, gin.H{"error": "cannot delete current admin"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete admin"})
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}
