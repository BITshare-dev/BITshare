package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/model"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type PublicUploadHandler struct {
	service *service.PublicUploadService
}

func NewPublicUploadHandler(service *service.PublicUploadService) *PublicUploadHandler {
	return &PublicUploadHandler{
		service: service,
	}
}

func (h *PublicUploadHandler) CreateSubmission(ctx *gin.Context) {
	request, err := h.parseSubmissionRequest(ctx)
	if err != nil {
		switch {
		case errors.Is(err, errUploadBodyTooLarge):
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload request exceeds limit"})
		case errors.Is(err, errUploadTotalTooLarge):
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "total upload size exceeds limit"})
		case errors.Is(err, errUploadFormParse):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse upload form"})
		case errors.Is(err, errUploadManifestInvalid):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload manifest"})
		case errors.Is(err, errUploadManifestMismatch):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "upload files do not match manifest"})
		case errors.Is(err, errUploadFileRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		case errors.Is(err, errUploadFileRead):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to read uploaded file"})
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse upload form"})
		}
		return
	}
	defer request.Close()

	requestCtx := ctx.Request.Context()
	if identity, ok := session.GetAdminIdentity(ctx); ok && identity.HasPermission(model.AdminPermissionSubmissionModeration) {
		requestCtx = service.WithPublicUploadActor(requestCtx, service.PublicUploadActor{
			AdminID:          identity.AdminID,
			CanDirectPublish: true,
		})
	}

	result, err := h.service.CreateSubmission(requestCtx, request.input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidUploadInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload form"})
		case errors.Is(err, service.ErrUploadFolderRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "target folder is required"})
		case errors.Is(err, service.ErrUploadFolderNotFound):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "target folder not found"})
		case errors.Is(err, service.ErrUploadReceiptExists):
			ctx.JSON(http.StatusConflict, gin.H{"error": "receipt code already exists"})
		case errors.Is(err, service.ErrUploadNameConflict):
			ctx.JSON(http.StatusConflict, gin.H{"error": "file or folder name already exists"})
		case errors.Is(err, service.ErrUploadTooLarge):
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "total upload size exceeds limit"})
		case errors.Is(err, service.ErrUploadEmptyFile):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is empty"})
		case errors.Is(err, service.ErrReceiptCodeGenerate):
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate receipt code"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create submission"})
		}
		return
	}

	writePublicReceiptCode(ctx, result.ReceiptCode)
	ctx.JSON(http.StatusCreated, result)
}
