package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

type FeedbackRepository struct {
	db *gorm.DB
}

func NewFeedbackRepository(db *gorm.DB) *FeedbackRepository {
	return &FeedbackRepository{db: db}
}

func (r *FeedbackRepository) Create(ctx context.Context, feedback *model.Feedback) error {
	return r.db.WithContext(ctx).Create(feedback).Error
}

func (r *FeedbackRepository) FileExists(ctx context.Context, fileID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.File{}).
		Where("id = ?", fileID).
		Count(&count).
		Error; err != nil {
		return false, fmt.Errorf("count file: %w", err)
	}
	return count > 0, nil
}

func (r *FeedbackRepository) FolderExists(ctx context.Context, folderID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Where("id = ?", folderID).
		Count(&count).
		Error; err != nil {
		return false, fmt.Errorf("count folder: %w", err)
	}
	return count > 0, nil
}

func (r *FeedbackRepository) FindFileNameByID(ctx context.Context, fileID string) (string, error) {
	var row struct {
		Name string
	}
	if err := r.db.WithContext(ctx).
		Model(&model.File{}).
		Select("name").
		Where("id = ?", fileID).
		Take(&row).
		Error; err != nil {
		return "", fmt.Errorf("find file name: %w", err)
	}
	return row.Name, nil
}

func (r *FeedbackRepository) FindFilePathByID(ctx context.Context, fileID string) (string, error) {
	var row struct {
		FolderID *string `gorm:"column:folder_id"`
		Name     string  `gorm:"column:name"`
	}
	if err := r.db.WithContext(ctx).
		Model(&model.File{}).
		Select("folder_id, name").
		Where("id = ?", fileID).
		Take(&row).
		Error; err != nil {
		return "", fmt.Errorf("find file path snapshot: %w", err)
	}

	folderPath, err := BuildFolderDisplayPath(ctx, r.db, row.FolderID)
	if err != nil {
		return "", err
	}
	return NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Join(folderPath, row.Name))), nil
}

func (r *FeedbackRepository) FindFolderNameByID(ctx context.Context, folderID string) (string, error) {
	var row struct {
		Name string
	}
	if err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Select("name").
		Where("id = ?", folderID).
		Take(&row).
		Error; err != nil {
		return "", fmt.Errorf("find folder name: %w", err)
	}
	return row.Name, nil
}

func (r *FeedbackRepository) FindFolderPathByID(ctx context.Context, folderID string) (string, error) {
	return BuildFolderDisplayPath(ctx, r.db, &folderID)
}

func (r *FeedbackRepository) FindByReceiptCode(ctx context.Context, receiptCode string) ([]model.Feedback, error) {
	var rows []model.Feedback
	if err := r.db.WithContext(ctx).
		Where("receipt_code = ?", strings.TrimSpace(receiptCode)).
		Order("created_at DESC").
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("find feedback by receipt code: %w", err)
	}
	return rows, nil
}

type FeedbackListRow struct {
	ID          string
	ReceiptCode string
	FileID      *string
	FolderID    *string
	TargetName  string
	TargetPath  string
	TargetType  string
	Description string
	ReporterIP  string
	CreatedAt   string `gorm:"column:created_at"`
}

func (r *FeedbackRepository) List(ctx context.Context) ([]model.Feedback, error) {
	var rows []model.Feedback
	if err := r.db.WithContext(ctx).
		Where("status = ?", model.FeedbackStatusPending).
		Order("created_at DESC").
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}
	return rows, nil
}

func (r *FeedbackRepository) FindByID(ctx context.Context, feedbackID string) (*model.Feedback, error) {
	var feedback model.Feedback
	if err := r.db.WithContext(ctx).Where("id = ?", feedbackID).Take(&feedback).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find feedback: %w", err)
	}
	return &feedback, nil
}

func (r *FeedbackRepository) Approve(ctx context.Context, feedbackID, adminID, operatorIP string, reviewedAt time.Time, reviewReason string) error {
	return r.review(ctx, feedbackID, adminID, operatorIP, reviewedAt, reviewReason, model.FeedbackStatusApproved, "feedback_approved")
}

func (r *FeedbackRepository) Reject(ctx context.Context, feedbackID, adminID, operatorIP string, reviewedAt time.Time, reviewReason string) error {
	return r.review(ctx, feedbackID, adminID, operatorIP, reviewedAt, reviewReason, model.FeedbackStatusRejected, "feedback_rejected")
}

func (r *FeedbackRepository) review(
	ctx context.Context,
	feedbackID string,
	adminID string,
	operatorIP string,
	reviewedAt time.Time,
	reviewReason string,
	status model.FeedbackStatus,
	action string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var feedback model.Feedback
		if err := tx.Where("id = ?", feedbackID).Take(&feedback).Error; err != nil {
			return fmt.Errorf("reload feedback: %w", err)
		}
		if feedback.Status != model.FeedbackStatusPending {
			return fmt.Errorf("feedback is not pending")
		}

		reviewerID := adminID
		if err := tx.Model(&model.Feedback{}).Where("id = ?", feedbackID).Updates(map[string]any{
			"status":        status,
			"review_reason": strings.TrimSpace(reviewReason),
			"reviewer_id":   &reviewerID,
			"reviewed_at":   &reviewedAt,
			"updated_at":    reviewedAt,
		}).Error; err != nil {
			return fmt.Errorf("update feedback review: %w", err)
		}

		if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{PendingFeedbacks: -1}); err != nil {
			return fmt.Errorf("adjust feedback stats: %w", err)
		}

		logID, err := identity.NewID()
		if err != nil {
			return fmt.Errorf("generate log id: %w", err)
		}

		targetType, targetID := feedbackTarget(&feedback)
		entry := &model.OperationLog{
			ID:         logID,
			AdminID:    &reviewerID,
			Action:     action,
			TargetType: targetType,
			TargetID:   targetID,
			Detail:     strings.TrimSpace(reviewReason),
			IP:         operatorIP,
			CreatedAt:  reviewedAt,
		}
		return tx.Create(entry).Error
	})
}

func feedbackTarget(feedback *model.Feedback) (string, string) {
	if feedback.FileID != nil {
		return "file", *feedback.FileID
	}
	if feedback.FolderID != nil {
		return "folder", *feedback.FolderID
	}
	return "feedback", feedback.ID
}
