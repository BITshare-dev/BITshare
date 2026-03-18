package handler

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

type PublicDownloadHandler struct {
	service *service.PublicDownloadService
}

type batchDownloadRequest struct {
	FileIDs []string `json:"file_ids"`
}

type resourceBatchDownloadRequest struct {
	FileIDs   []string `json:"file_ids"`
	FolderIDs []string `json:"folder_ids"`
}

func NewPublicDownloadHandler(service *service.PublicDownloadService) *PublicDownloadHandler {
	return &PublicDownloadHandler{service: service}
}

func (h *PublicDownloadHandler) DownloadFile(ctx *gin.Context) {
	download, err := h.service.PrepareDownload(ctx.Request.Context(), ctx.Param("fileID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "file is unavailable"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download file"})
		}
		return
	}
	defer download.Content.Close()

	if download.MimeType != "" {
		ctx.Header("Content-Type", download.MimeType)
	}
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.OriginalName))
	ctx.Header("Content-Length", strconv.FormatInt(download.Size, 10))

	if err := h.service.RecordDownload(ctx.Request.Context(), download.FileID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	http.ServeContent(ctx.Writer, ctx.Request, download.OriginalName, download.ModTime, download.Content)
}

func (h *PublicDownloadHandler) DownloadFolder(ctx *gin.Context) {
	download, err := h.service.PrepareFolderDownload(ctx.Request.Context(), ctx.Param("folderID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "one or more files are unavailable"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download folder"})
		}
		return
	}

	fileIDs := make([]string, 0, len(download.Items))
	for _, item := range download.Items {
		fileIDs = append(fileIDs, item.FileID)
	}
	if err := h.service.RecordBatchDownload(ctx.Request.Context(), fileIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.FolderName+".zip"))
	zipWriter := zip.NewWriter(ctx.Writer)
	usedNames := make(map[string]int, len(download.Items))

	for _, item := range download.Items {
		opened, openErr := h.service.PrepareDownload(ctx.Request.Context(), item.FileID)
		if openErr != nil {
			zipWriter.Close()
			return
		}

		entryName := uniqueZipEntryName(item.ZipPath, usedNames)
		entry, createErr := zipWriter.Create(entryName)
		if createErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		if _, copyErr := io.Copy(entry, opened.Content); copyErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		opened.Content.Close()
	}
	_ = zipWriter.Close()
}

func (h *PublicDownloadHandler) GetFileDetail(ctx *gin.Context) {
	detail, err := h.service.GetFileDetail(ctx.Request.Context(), ctx.Param("fileID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load file detail"})
		}
		return
	}
	ctx.JSON(http.StatusOK, detail)
}

func (h *PublicDownloadHandler) DownloadBatch(ctx *gin.Context) {
	var req batchDownloadRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	files, err := h.service.PrepareBatchDownload(ctx.Request.Context(), req.FileIDs)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBatchDownloadInvalid):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file_ids is required"})
		case errors.Is(err, service.ErrDownloadFileNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "one or more files were not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "one or more files are unavailable"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare batch download"})
		}
		return
	}

	fileIDs := make([]string, 0, len(files))
	for _, item := range files {
		fileIDs = append(fileIDs, item.FileID)
	}
	if err := h.service.RecordBatchDownload(ctx.Request.Context(), fileIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", `attachment; filename="openshare-batch.zip"`)
	zipWriter := zip.NewWriter(ctx.Writer)
	usedNames := make(map[string]int, len(files))

	for _, item := range files {
		opened, openErr := h.service.PrepareDownload(ctx.Request.Context(), item.FileID)
		if openErr != nil {
			zipWriter.Close()
			return
		}

		entryName := uniqueZipEntryName(item.OriginalName, usedNames)
		entry, createErr := zipWriter.Create(entryName)
		if createErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		if _, copyErr := io.Copy(entry, opened.Content); copyErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		opened.Content.Close()
	}
	_ = zipWriter.Close()
}

func (h *PublicDownloadHandler) DownloadResourceBatch(ctx *gin.Context) {
	var req resourceBatchDownloadRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	files, err := h.service.PrepareResourceBatchDownload(ctx.Request.Context(), req.FileIDs, req.FolderIDs)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBatchDownloadInvalid):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file_ids or folder_ids is required"})
		case errors.Is(err, service.ErrDownloadFileNotFound), errors.Is(err, service.ErrDownloadFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "one or more resources were not found"})
		case errors.Is(err, service.ErrDownloadFileUnavailable):
			ctx.JSON(http.StatusGone, gin.H{"error": "one or more files are unavailable"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare batch download"})
		}
		return
	}

	fileIDs := make([]string, 0, len(files))
	for _, item := range files {
		fileIDs = append(fileIDs, item.FileID)
	}
	if err := h.service.RecordBatchDownload(ctx.Request.Context(), fileIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record download"})
		return
	}

	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", `attachment; filename="openshare-selection.zip"`)
	zipWriter := zip.NewWriter(ctx.Writer)
	usedNames := make(map[string]int, len(files))

	for _, item := range files {
		opened, openErr := h.service.PrepareDownload(ctx.Request.Context(), item.FileID)
		if openErr != nil {
			zipWriter.Close()
			return
		}

		entryName := uniqueZipEntryName(item.ZipPath, usedNames)
		entry, createErr := zipWriter.Create(entryName)
		if createErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		if _, copyErr := io.Copy(entry, opened.Content); copyErr != nil {
			opened.Content.Close()
			zipWriter.Close()
			return
		}
		opened.Content.Close()
	}
	_ = zipWriter.Close()
}

func uniqueZipEntryName(originalName string, used map[string]int) string {
	originalName = strings.TrimSpace(originalName)
	if originalName == "" {
		originalName = "file"
	}
	if _, exists := used[originalName]; !exists {
		used[originalName] = 1
		return originalName
	}

	ext := ""
	base := originalName
	if dot := strings.LastIndex(originalName, "."); dot > 0 {
		base = originalName[:dot]
		ext = originalName[dot:]
	}
	next := used[originalName]
	used[originalName] = next + 1
	return fmt.Sprintf("%s_%d%s", base, next, ext)
}
