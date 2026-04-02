package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

var (
	ErrFeedbackNotFound             = errors.New("feedback not found")
	ErrFeedbackNotPending           = errors.New("feedback is not pending")
	ErrFeedbackDescriptionRequired  = errors.New("feedback description is required")
	ErrFeedbackTargetRequired       = errors.New("exactly one of file_id or folder_id is required")
	ErrFeedbackTargetNotFound       = errors.New("feedback target not found")
	ErrFeedbackReviewReasonRequired = errors.New("feedback review reason is required")
)

type FeedbackService struct {
	repository   *repository.FeedbackRepository
	receiptCodes *ReceiptCodeService
	nowFunc      func() time.Time
}

func NewFeedbackService(repository *repository.FeedbackRepository, receiptCodes *ReceiptCodeService) *FeedbackService {
	return &FeedbackService{
		repository:   repository,
		receiptCodes: receiptCodes,
		nowFunc:      func() time.Time { return time.Now().UTC() },
	}
}

type CreateFeedbackInput struct {
	FileID      string
	FolderID    string
	ReceiptCode string
	Description string
	ReporterIP  string
}

type CreateFeedbackResult struct {
	FeedbackID  string    `json:"feedback_id"`
	ReceiptCode string    `json:"receipt_code"`
	CreatedAt   time.Time `json:"created_at"`
}

type PublicFeedbackLookupResult struct {
	ReceiptCode string                     `json:"receipt_code"`
	Items       []PublicFeedbackLookupItem `json:"items"`
}

type PublicFeedbackLookupItem struct {
	TargetName   string     `json:"target_name"`
	TargetPath   string     `json:"target_path"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	ReviewReason string     `json:"review_reason"`
	CreatedAt    time.Time  `json:"created_at"`
	ReviewedAt   *time.Time `json:"reviewed_at"`
}

type FeedbackItem struct {
	ID          string    `json:"id"`
	ReceiptCode string    `json:"receipt_code"`
	FileID      *string   `json:"file_id"`
	FolderID    *string   `json:"folder_id"`
	TargetName  string    `json:"target_name"`
	TargetPath  string    `json:"target_path"`
	TargetType  string    `json:"target_type"`
	Description string    `json:"description"`
	ReporterIP  string    `json:"reporter_ip"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type ReviewFeedbackResult struct {
	FeedbackID string    `json:"feedback_id"`
	Status     string    `json:"status"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

func (s *FeedbackService) Create(ctx context.Context, input CreateFeedbackInput) (*CreateFeedbackResult, error) {
	description := strings.TrimSpace(input.Description)
	if description == "" {
		return nil, ErrFeedbackDescriptionRequired
	}

	hasFile := strings.TrimSpace(input.FileID) != ""
	hasFolder := strings.TrimSpace(input.FolderID) != ""
	if hasFile == hasFolder {
		return nil, ErrFeedbackTargetRequired
	}

	targetName := ""
	targetPath := ""
	targetType := ""

	if hasFile {
		exists, err := s.repository.FileExists(ctx, strings.TrimSpace(input.FileID))
		if err != nil {
			return nil, fmt.Errorf("check file existence: %w", err)
		}
		if !exists {
			return nil, ErrFeedbackTargetNotFound
		}
		targetName, err = s.repository.FindFileNameByID(ctx, strings.TrimSpace(input.FileID))
		if err != nil {
			return nil, fmt.Errorf("load file name snapshot: %w", err)
		}
		targetPath, err = s.repository.FindFilePathByID(ctx, strings.TrimSpace(input.FileID))
		if err != nil {
			return nil, fmt.Errorf("load file path snapshot: %w", err)
		}
		targetType = "file"
	} else {
		exists, err := s.repository.FolderExists(ctx, strings.TrimSpace(input.FolderID))
		if err != nil {
			return nil, fmt.Errorf("check folder existence: %w", err)
		}
		if !exists {
			return nil, ErrFeedbackTargetNotFound
		}
		targetName, err = s.repository.FindFolderNameByID(ctx, strings.TrimSpace(input.FolderID))
		if err != nil {
			return nil, fmt.Errorf("load folder name snapshot: %w", err)
		}
		targetPath, err = s.repository.FindFolderPathByID(ctx, strings.TrimSpace(input.FolderID))
		if err != nil {
			return nil, fmt.Errorf("load folder path snapshot: %w", err)
		}
		targetType = "folder"
	}

	receiptCode, err := s.receiptCodes.ResolveForSession(ctx, input.ReceiptCode)
	if err != nil {
		return nil, err
	}

	feedbackID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate feedback id: %w", err)
	}

	now := s.nowFunc()
	feedback := &model.Feedback{
		ID:          feedbackID,
		ReceiptCode: receiptCode,
		TargetName:  targetName,
		TargetPath:  targetPath,
		TargetType:  targetType,
		Description: description,
		ReporterIP:  input.ReporterIP,
		Status:      model.FeedbackStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if hasFile {
		fileID := strings.TrimSpace(input.FileID)
		feedback.FileID = &fileID
	} else {
		folderID := strings.TrimSpace(input.FolderID)
		feedback.FolderID = &folderID
	}

	if err := s.repository.Create(ctx, feedback); err != nil {
		return nil, fmt.Errorf("create feedback: %w", err)
	}

	return &CreateFeedbackResult{
		FeedbackID:  feedbackID,
		ReceiptCode: receiptCode,
		CreatedAt:   now,
	}, nil
}

func (s *FeedbackService) LookupByReceiptCode(ctx context.Context, receiptCode string) (*PublicFeedbackLookupResult, error) {
	normalized, err := normalizeReceiptCode(receiptCode)
	if err != nil {
		return nil, ErrInvalidUploadInput
	}

	items, err := s.repository.FindByReceiptCode(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("lookup feedback: %w", err)
	}
	if len(items) == 0 {
		return nil, ErrFeedbackNotFound
	}

	resultItems := make([]PublicFeedbackLookupItem, 0, len(items))
	for _, item := range items {
		resultItems = append(resultItems, PublicFeedbackLookupItem{
			TargetName:   item.TargetName,
			TargetPath:   item.TargetPath,
			Description:  item.Description,
			Status:       string(item.Status),
			ReviewReason: item.ReviewReason,
			CreatedAt:    item.CreatedAt,
			ReviewedAt:   item.ReviewedAt,
		})
	}

	return &PublicFeedbackLookupResult{
		ReceiptCode: normalized,
		Items:       resultItems,
	}, nil
}

func (s *FeedbackService) List(ctx context.Context) ([]FeedbackItem, error) {
	rows, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}

	items := make([]FeedbackItem, 0, len(rows))
	for _, row := range rows {
		fileID := row.FileID
		folderID := row.FolderID
		items = append(items, FeedbackItem{
			ID:          row.ID,
			ReceiptCode: row.ReceiptCode,
			FileID:      fileID,
			FolderID:    folderID,
			TargetName:  row.TargetName,
			TargetPath:  row.TargetPath,
			TargetType:  row.TargetType,
			Description: row.Description,
			ReporterIP:  row.ReporterIP,
			Status:      string(row.Status),
			CreatedAt:   row.CreatedAt,
		})
	}
	return items, nil
}

func (s *FeedbackService) Approve(ctx context.Context, feedbackID, adminID, operatorIP, reviewReason string) (*ReviewFeedbackResult, error) {
	feedback, err := s.repository.FindByID(ctx, strings.TrimSpace(feedbackID))
	if err != nil {
		return nil, fmt.Errorf("find feedback: %w", err)
	}
	if feedback == nil {
		return nil, ErrFeedbackNotFound
	}
	if feedback.Status != model.FeedbackStatusPending {
		return nil, ErrFeedbackNotPending
	}

	reviewedAt := s.nowFunc()
	if err := s.repository.Approve(ctx, feedback.ID, adminID, operatorIP, reviewedAt, strings.TrimSpace(reviewReason)); err != nil {
		return nil, fmt.Errorf("approve feedback: %w", err)
	}

	return &ReviewFeedbackResult{
		FeedbackID: feedback.ID,
		Status:     string(model.FeedbackStatusApproved),
		ReviewedAt: reviewedAt,
	}, nil
}

func (s *FeedbackService) Reject(ctx context.Context, feedbackID, adminID, operatorIP, reviewReason string) (*ReviewFeedbackResult, error) {
	reviewReason = strings.TrimSpace(reviewReason)
	if reviewReason == "" {
		return nil, ErrFeedbackReviewReasonRequired
	}

	feedback, err := s.repository.FindByID(ctx, strings.TrimSpace(feedbackID))
	if err != nil {
		return nil, fmt.Errorf("find feedback: %w", err)
	}
	if feedback == nil {
		return nil, ErrFeedbackNotFound
	}
	if feedback.Status != model.FeedbackStatusPending {
		return nil, ErrFeedbackNotPending
	}

	reviewedAt := s.nowFunc()
	if err := s.repository.Reject(ctx, feedback.ID, adminID, operatorIP, reviewedAt, reviewReason); err != nil {
		return nil, fmt.Errorf("reject feedback: %w", err)
	}

	return &ReviewFeedbackResult{
		FeedbackID: feedback.ID,
		Status:     string(model.FeedbackStatusRejected),
		ReviewedAt: reviewedAt,
	}, nil
}
