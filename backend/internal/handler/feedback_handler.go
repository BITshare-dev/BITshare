package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

type FeedbackHandler struct {
	service *service.FeedbackService
}

func NewFeedbackHandler(service *service.FeedbackService) *FeedbackHandler {
	return &FeedbackHandler{service: service}
}

type createFeedbackRequest struct {
	FileID      string `json:"file_id"`
	FolderID    string `json:"folder_id"`
	Description string `json:"description"`
}

func (h *FeedbackHandler) Create(ctx *gin.Context) {
	var req createFeedbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	result, err := h.service.Create(ctx.Request.Context(), service.CreateFeedbackInput{
		FileID:      req.FileID,
		FolderID:    req.FolderID,
		ReceiptCode: readPublicReceiptCode(ctx),
		Description: req.Description,
		ReporterIP:  ctx.ClientIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFeedbackDescriptionRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		case errors.Is(err, service.ErrFeedbackTargetRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "exactly one of file_id or folder_id is required"})
		case errors.Is(err, service.ErrFeedbackTargetNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "feedback target not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create feedback"})
		}
		return
	}

	writePublicReceiptCode(ctx, result.ReceiptCode)
	ctx.JSON(http.StatusCreated, result)
}

func (h *FeedbackHandler) LookupByReceiptCode(ctx *gin.Context) {
	result, err := h.service.LookupByReceiptCode(ctx.Request.Context(), ctx.Param("receiptCode"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidUploadInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid receipt code"})
		case errors.Is(err, service.ErrFeedbackNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "feedback not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query feedback"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (h *FeedbackHandler) List(ctx *gin.Context) {
	items, err := h.service.List(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list feedback"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

type reviewFeedbackRequest struct {
	ReviewReason string `json:"review_reason"`
}

func (h *FeedbackHandler) Approve(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req reviewFeedbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		req = reviewFeedbackRequest{}
	}

	result, err := h.service.Approve(ctx.Request.Context(), ctx.Param("feedbackID"), identity.AdminID, ctx.ClientIP(), req.ReviewReason)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFeedbackNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "feedback not found"})
		case errors.Is(err, service.ErrFeedbackNotPending):
			ctx.JSON(http.StatusConflict, gin.H{"error": "feedback is not pending"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve feedback"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}

func (h *FeedbackHandler) Reject(ctx *gin.Context) {
	identity, ok := session.GetAdminIdentity(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req reviewFeedbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		req = reviewFeedbackRequest{}
	}

	result, err := h.service.Reject(ctx.Request.Context(), ctx.Param("feedbackID"), identity.AdminID, ctx.ClientIP(), req.ReviewReason)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFeedbackReviewReasonRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "review_reason is required"})
		case errors.Is(err, service.ErrFeedbackNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "feedback not found"})
		case errors.Is(err, service.ErrFeedbackNotPending):
			ctx.JSON(http.StatusConflict, gin.H{"error": "feedback is not pending"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject feedback"})
		}
		return
	}

	ctx.JSON(http.StatusOK, result)
}
