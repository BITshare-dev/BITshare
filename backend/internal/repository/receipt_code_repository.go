package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type ReceiptCodeRepository struct {
	db *gorm.DB
}

func NewReceiptCodeRepository(db *gorm.DB) *ReceiptCodeRepository {
	return &ReceiptCodeRepository{db: db}
}

func (r *ReceiptCodeRepository) Exists(ctx context.Context, receiptCode string) (bool, error) {
	var submissionCount int64
	if err := r.db.WithContext(ctx).
		Table("submissions").
		Where("receipt_code = ?", receiptCode).
		Count(&submissionCount).
		Error; err != nil {
		return false, fmt.Errorf("count submissions by receipt code: %w", err)
	}
	if submissionCount > 0 {
		return true, nil
	}

	var feedbackCount int64
	if err := r.db.WithContext(ctx).
		Model(&model.Feedback{}).
		Where("receipt_code = ?", receiptCode).
		Count(&feedbackCount).
		Error; err != nil {
		return false, fmt.Errorf("count feedback by receipt code: %w", err)
	}
	return feedbackCount > 0, nil
}
