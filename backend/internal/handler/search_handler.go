package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

// SearchHandler exposes the public search API.
type SearchHandler struct {
	service *service.SearchService
}

func NewSearchHandler(service *service.SearchService) *SearchHandler {
	return &SearchHandler{service: service}
}

// Search handles GET /api/public/search
//
//	Query parameters:
//	  q         – keyword
//	  folder_id – optional folder scope
//	  page      – page number (default 1)
//	  page_size – results per page (default 20, max 100)
func (h *SearchHandler) Search(ctx *gin.Context) {
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

	result, err := h.service.Search(ctx.Request.Context(), service.SearchInput{
		Keyword:  ctx.Query("q"),
		FolderID: ctx.Query("folder_id"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSearchQueryEmpty):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "search query is empty"})
		case errors.Is(err, service.ErrSearchQueryTooLong):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "search query is too long"})
		case errors.Is(err, service.ErrSearchInvalidInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid search parameters"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}
