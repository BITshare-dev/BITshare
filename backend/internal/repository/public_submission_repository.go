package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type PublicSubmissionRepository struct {
	db *gorm.DB
}

type SubmissionLookupRow struct {
	ReceiptCode  string
	FolderID     *string `gorm:"column:folder_id"`
	Name         string
	RelativePath string
	Status       model.SubmissionStatus
	ReviewReason string `gorm:"column:review_reason"`
	CreatedAt    time.Time
}

func NewPublicSubmissionRepository(db *gorm.DB) *PublicSubmissionRepository {
	return &PublicSubmissionRepository{db: db}
}

func (r *PublicSubmissionRepository) FindAllByReceiptCode(ctx context.Context, receiptCode string) ([]SubmissionLookupRow, error) {
	var rows []SubmissionLookupRow
	err := r.db.WithContext(ctx).
		Table("submissions").
		Select(`
				submissions.receipt_code AS receipt_code,
				submissions.folder_id AS folder_id,
				submissions.name AS name,
				submissions.relative_path AS relative_path,
				submissions.status AS status,
				submissions.review_reason AS review_reason,
				submissions.created_at AS created_at
			`).
		Where("submissions.receipt_code = ?", receiptCode).
		Order("submissions.created_at DESC").
		Find(&rows).
		Error
	if err != nil {
		return nil, fmt.Errorf("find submissions by receipt code: %w", err)
	}

	return rows, nil
}

func (r *PublicSubmissionRepository) BuildFolderDisplayPath(ctx context.Context, folderID *string) (string, error) {
	return BuildFolderDisplayPath(ctx, r.db, folderID)
}
