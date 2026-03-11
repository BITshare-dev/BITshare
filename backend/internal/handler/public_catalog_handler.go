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

func (h *PublicCatalogHandler) ListPublicFiles(ctx *gin.Context) {
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

	result, err := h.service.ListPublicFiles(ctx.Request.Context(), service.PublicFileListInput{
		FolderID: ctx.Query("folder_id"),
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

	ctx.JSON(http.StatusOK, gin.H{
		"items":     result.Items,
		"page":      result.Page,
		"page_size": result.PageSize,
		"total":     result.Total,
	})
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
