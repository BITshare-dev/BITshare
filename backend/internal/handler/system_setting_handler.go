package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type SystemSettingHandler struct {
	service *service.SystemSettingService
}

func NewSystemSettingHandler(service *service.SystemSettingService) *SystemSettingHandler {
	return &SystemSettingHandler{service: service}
}

func (h *SystemSettingHandler) GetPolicy(ctx *gin.Context) {
	policy, err := h.service.GetPolicy(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load system settings"})
		return
	}
	ctx.JSON(http.StatusOK, policy)
}

func (h *SystemSettingHandler) SavePolicy(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req service.SystemPolicy
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	policy, err := h.service.SavePolicy(ctx.Request.Context(), req, identity.AdminID, ctx.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrInvalidUploadInput) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid system settings"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save system settings"})
		return
	}
	ctx.JSON(http.StatusOK, policy)
}
