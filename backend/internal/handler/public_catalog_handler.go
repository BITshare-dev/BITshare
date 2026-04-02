package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

type PublicCatalogHandler struct {
	service *service.PublicCatalogService
}

func NewPublicCatalogHandler(service *service.PublicCatalogService) *PublicCatalogHandler {
	return &PublicCatalogHandler{service: service}
}

func (h *PublicCatalogHandler) ListPublicFolderFiles(ctx *gin.Context) {
	page, err := parseIntQuery(ctx.Query("page"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
		return
	}

	pageSize, err := parseIntQuery(ctx.Query("page_size"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid page_size"})
		return
	}

	result, err := h.service.ListPublicFolderFiles(ctx.Request.Context(), service.PublicFolderFileListInput{
		FolderID: ctx.Param("folderID"),
		Page:     page,
		PageSize: pageSize,
		Sort:     ctx.Query("sort"),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidPublicFileQuery):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		case errors.Is(err, service.ErrFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list public files"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (h *PublicCatalogHandler) ListHotFiles(ctx *gin.Context) {
	limit, err := parseIntQuery(ctx.Query("limit"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
		return
	}

	result, err := h.service.ListHotFiles(ctx.Request.Context(), limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list hot files"})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *PublicCatalogHandler) ListLatestFiles(ctx *gin.Context) {
	limit, err := parseIntQuery(ctx.Query("limit"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
		return
	}

	result, err := h.service.ListLatestFiles(ctx.Request.Context(), limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list latest files"})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *PublicCatalogHandler) ListPublicFolders(ctx *gin.Context) {
	items, err := h.service.ListPublicFolders(ctx.Request.Context(), ctx.Query("parent_id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list folders"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *PublicCatalogHandler) GetPublicFolderDetail(ctx *gin.Context) {
	detail, err := h.service.GetPublicFolderDetail(ctx.Request.Context(), ctx.Param("folderID"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFolderNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get folder detail"})
		}
		return
	}

	ctx.JSON(http.StatusOK, detail)
}

func parseIntQuery(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	return value, nil
}
