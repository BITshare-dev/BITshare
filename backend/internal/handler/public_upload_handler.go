package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

type PublicUploadHandler struct {
	service         *service.PublicUploadService
	maxRequestBytes int64
}

func NewPublicUploadHandler(service *service.PublicUploadService, maxRequestBytes int64) *PublicUploadHandler {
	return &PublicUploadHandler{
		service:         service,
		maxRequestBytes: maxRequestBytes,
	}
}

func (h *PublicUploadHandler) CreateSubmission(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, h.maxRequestBytes)

	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to read uploaded file"})
		return
	}
	defer file.Close()

	result, err := h.service.CreateSubmission(ctx.Request.Context(), service.PublicUploadInput{
		Description:  ctx.PostForm("description"),
		Tags:         append(ctx.PostFormArray("tag"), ctx.PostFormArray("tags")...),
		ReceiptCode:  ctx.PostForm("receipt_code"),
		FolderID:     ctx.PostForm("folder_id"),
		OriginalName: fileHeader.Filename,
		DeclaredMIME: fileHeader.Header.Get("Content-Type"),
		UploaderIP:   ctx.ClientIP(),
		File:         file,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidUploadInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload form"})
		case errors.Is(err, service.ErrUploadFolderNotFound):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "target folder not found"})
		case errors.Is(err, service.ErrUploadReceiptExists):
			ctx.JSON(http.StatusConflict, gin.H{"error": "receipt code already exists"})
		case errors.Is(err, service.ErrUploadFileTooLarge):
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file is too large"})
		case errors.Is(err, service.ErrUploadEmptyFile):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is empty"})
		case errors.Is(err, service.ErrInvalidFileExtension):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file extension is not allowed"})
		case errors.Is(err, service.ErrInvalidFileMIMEType):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file type is not allowed"})
		case errors.Is(err, service.ErrReceiptCodeGenerate):
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate receipt code"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create submission"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, result)
}
