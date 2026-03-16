package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

type SiteVisitHandler struct {
	service *service.SiteVisitService
}

type recordSiteVisitRequest struct {
	Scope string `json:"scope"`
	Path  string `json:"path"`
}

func NewSiteVisitHandler(service *service.SiteVisitService) *SiteVisitHandler {
	return &SiteVisitHandler{service: service}
}

func (h *SiteVisitHandler) Record(ctx *gin.Context) {
	var req recordSiteVisitRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := h.service.Record(ctx.Request.Context(), req.Scope, req.Path, ctx.ClientIP()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record site visit"})
		return
	}
	ctx.Status(http.StatusNoContent)
}
