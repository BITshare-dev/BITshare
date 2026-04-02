package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type ModerationHandler struct {
	service *service.ModerationService
}

type reviewSubmissionRequest struct {
	ReviewReason string `json:"review_reason"`
	RejectReason string `json:"reject_reason"`
}

func NewModerationHandler(service *service.ModerationService) *ModerationHandler {
	return &ModerationHandler{service: service}
}

func (h *ModerationHandler) ListPendingSubmissions(ctx *gin.Context) {
	items, err := h.service.ListPendingSubmissions(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pending submissions"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *ModerationHandler) ApproveSubmission(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	result, err := h.service.ApproveSubmission(ctx.Request.Context(), ctx.Param("submissionID"), identity.AdminID, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSubmissionMissing):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		case errors.Is(err, service.ErrSubmissionNotPending):
			ctx.JSON(http.StatusConflict, gin.H{"error": "submission is not pending"})
		case errors.Is(err, service.ErrStagedFileMissing):
			ctx.JSON(http.StatusConflict, gin.H{"error": "staged file is missing"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve submission"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (h *ModerationHandler) RejectSubmission(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req reviewSubmissionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	reviewReason := req.ReviewReason
	if reviewReason == "" {
		reviewReason = req.RejectReason
	}

	result, err := h.service.RejectSubmission(ctx.Request.Context(), ctx.Param("submissionID"), identity.AdminID, ctx.ClientIP(), reviewReason)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSubmissionReviewReasonRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "review_reason is required"})
		case errors.Is(err, service.ErrSubmissionMissing):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		case errors.Is(err, service.ErrSubmissionNotPending):
			ctx.JSON(http.StatusConflict, gin.H{"error": "submission is not pending"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject submission"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}
