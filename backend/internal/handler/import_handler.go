package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type ImportHandler struct {
	service     *service.ImportService
	authService *service.AdminAuthService
}

type importLocalRequest struct {
	RootPath string `json:"root_path"`
}

type deleteManagedDirectoryRequest struct {
	Password string `json:"password"`
}

func (h *ImportHandler) ListDirectories(ctx *gin.Context) {
	result, err := h.service.ListDirectories(ctx.Request.Context(), ctx.Query("path"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImportPath):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid import path"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to browse import directories"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func NewImportHandler(service *service.ImportService, authService *service.AdminAuthService) *ImportHandler {
	return &ImportHandler{service: service, authService: authService}
}

func (h *ImportHandler) ImportLocalDirectory(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req importLocalRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	result, err := h.service.ImportLocalDirectory(ctx.Request.Context(), service.LocalImportInput{
		RootPath:   req.RootPath,
		AdminID:    identity.AdminID,
		OperatorIP: ctx.ClientIP(),
	})
	if err != nil {
		var conflictErr *service.ManagedDirectoryConflictError
		switch {
		case errors.Is(err, service.ErrInvalidImportPath):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid import path"})
		case errors.As(err, &conflictErr):
			ctx.JSON(http.StatusConflict, gin.H{"error": conflictErr.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import local directory"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (h *ImportHandler) RescanManagedDirectory(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	result, err := h.service.RescanManagedDirectory(ctx.Request.Context(), ctx.Param("folderID"), identity.AdminID, ctx.ClientIP())
	if err != nil {
		var unavailableErr *service.ManagedDirectoryUnavailableError
		switch {
		case errors.Is(err, service.ErrFolderTreeNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrManagedRootRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "managed root folder required"})
		case errors.As(err, &unavailableErr):
			ctx.JSON(http.StatusConflict, gin.H{"error": unavailableErr.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rescan managed directory"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (h *ImportHandler) GetFolderTree(ctx *gin.Context) {
	tree, err := h.service.GetFolderTree(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load folder tree"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": tree})
}

func (h *ImportHandler) DeleteManagedDirectory(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	if !identity.IsSuperAdmin() {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "super admin required"})
		return
	}

	var req deleteManagedDirectoryRequest
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

	if err := h.service.DeleteManagedDirectory(ctx.Request.Context(), ctx.Param("folderID"), identity.AdminID, ctx.ClientIP()); err != nil {
		switch {
		case errors.Is(err, service.ErrFolderTreeNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrManagedRootRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "managed root folder required"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete managed directory"})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}
