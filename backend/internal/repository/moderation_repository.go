package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

type ModerationRepository struct {
	db *gorm.DB
}

type PendingSubmissionRow struct {
	SubmissionID string
	ReceiptCode  string
	FolderID     *string `gorm:"column:folder_id"`
	Name         string
	Description  string
	RelativePath string
	ReviewReason string `gorm:"column:review_reason"`
	Status       model.SubmissionStatus
	CreatedAt    time.Time
	Size         int64
	MimeType     string
}

type PendingSubmissionRecord struct {
	Submission model.Submission
}

func NewModerationRepository(db *gorm.DB) *ModerationRepository {
	return &ModerationRepository{db: db}
}

func (r *ModerationRepository) ListPendingSubmissions(ctx context.Context) ([]PendingSubmissionRow, error) {
	var rows []PendingSubmissionRow
	err := r.db.WithContext(ctx).
		Table("submissions").
		Select(`
				submissions.id AS submission_id,
				submissions.receipt_code AS receipt_code,
				submissions.folder_id AS folder_id,
				submissions.name AS name,
				submissions.description AS description,
				submissions.relative_path AS relative_path,
				submissions.review_reason AS review_reason,
				submissions.status AS status,
				submissions.created_at AS created_at,
			submissions.size AS size,
			submissions.mime_type AS mime_type
		`).
		Where("submissions.status = ?", model.SubmissionStatusPending).
		Order("submissions.created_at DESC").
		Scan(&rows).
		Error
	if err != nil {
		return nil, fmt.Errorf("list pending submissions: %w", err)
	}

	return rows, nil
}

func (r *ModerationRepository) FindPendingSubmission(ctx context.Context, submissionID string) (*PendingSubmissionRecord, error) {
	var submission model.Submission
	err := r.db.WithContext(ctx).
		Where("id = ?", submissionID).
		Take(&submission).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find submission: %w", err)
	}

	return &PendingSubmissionRecord{Submission: submission}, nil
}

func (r *ModerationRepository) FindFolderByID(ctx context.Context, folderID string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).Where("id = ?", folderID).Take(&folder).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find folder: %w", err)
	}
	return &folder, nil
}

func (r *ModerationRepository) DB() *gorm.DB {
	return r.db
}

func (r *ModerationRepository) BuildFolderDisplayPath(ctx context.Context, folderID *string) (string, error) {
	return BuildFolderDisplayPath(ctx, r.db, folderID)
}

func (r *ModerationRepository) ApproveSubmission(
	ctx context.Context,
	submissionID string,
	adminID string,
	operatorIP string,
	reviewedAt time.Time,
	targetFolderID string,
	finalFileName string,
	finalRelativePath string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var submission model.Submission
		if err := tx.Where("id = ?", submissionID).Take(&submission).Error; err != nil {
			return fmt.Errorf("reload submission: %w", err)
		}
		if submission.Status != model.SubmissionStatusPending {
			return fmt.Errorf("submission is not pending")
		}

		reviewerID := adminID
		fileID, err := newOperationLogID()
		if err != nil {
			return fmt.Errorf("generate approved file id: %w", err)
		}
		file := &model.File{
			ID:            fileID,
			FolderID:      &targetFolderID,
			Name:          finalFileName,
			Description:   submission.Description,
			Extension:     submission.Extension,
			MimeType:      submission.MimeType,
			Size:          submission.Size,
			DownloadCount: 0,
			CreatedAt:     submission.CreatedAt,
			UpdatedAt:     reviewedAt,
		}
		if err := tx.Create(file).Error; err != nil {
			return fmt.Errorf("create approved file: %w", err)
		}

		if err := tx.Model(&model.Submission{}).
			Where("id = ?", submissionID).
			Updates(map[string]any{
				"name":          finalFileName,
				"relative_path": finalRelativePath,
				"file_id":       fileID,
				"status":        model.SubmissionStatusApproved,
				"review_reason": "",
				"staging_path":  "",
				"reviewer_id":   &reviewerID,
				"reviewed_at":   &reviewedAt,
				"updated_at":    reviewedAt,
			}).Error; err != nil {
			return fmt.Errorf("approve submission: %w", err)
		}

		if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{
			PendingSubmissions: -1,
		}); err != nil {
			return fmt.Errorf("adjust approved submission system stats: %w", err)
		}

		logID, err := newOperationLogID()
		if err != nil {
			return fmt.Errorf("generate operation log id: %w", err)
		}
		entry := &model.OperationLog{
			ID:         logID,
			AdminID:    &reviewerID,
			Action:     "submission_approved",
			TargetType: "submission",
			TargetID:   submissionID,
			Detail:     finalFileName,
			IP:         operatorIP,
			CreatedAt:  reviewedAt,
		}
		if err := tx.Create(entry).Error; err != nil {
			return fmt.Errorf("create operation log: %w", err)
		}

		return nil
	})
}

func (r *ModerationRepository) RejectSubmission(
	ctx context.Context,
	submissionID string,
	adminID string,
	operatorIP string,
	reviewedAt time.Time,
	rejectReason string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var submission model.Submission
		if err := tx.Where("id = ?", submissionID).Take(&submission).Error; err != nil {
			return fmt.Errorf("reload submission: %w", err)
		}
		if submission.Status != model.SubmissionStatusPending {
			return fmt.Errorf("submission is not pending")
		}

		reviewerID := adminID
		if err := tx.Model(&model.Submission{}).
			Where("id = ?", submissionID).
			Updates(map[string]any{
				"status":        model.SubmissionStatusRejected,
				"review_reason": rejectReason,
				"staging_path":  "",
				"reviewer_id":   &reviewerID,
				"reviewed_at":   &reviewedAt,
				"updated_at":    reviewedAt,
			}).Error; err != nil {
			return fmt.Errorf("reject submission: %w", err)
		}

		if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{PendingSubmissions: -1}); err != nil {
			return fmt.Errorf("adjust rejected submission system stats: %w", err)
		}

		logID, err := newOperationLogID()
		if err != nil {
			return fmt.Errorf("generate operation log id: %w", err)
		}
		entry := &model.OperationLog{
			ID:         logID,
			AdminID:    &reviewerID,
			Action:     "submission_rejected",
			TargetType: "submission",
			TargetID:   submissionID,
			Detail:     rejectReason,
			IP:         operatorIP,
			CreatedAt:  reviewedAt,
		}
		if err := tx.Create(entry).Error; err != nil {
			return fmt.Errorf("create operation log: %w", err)
		}

		return nil
	})
}

func newOperationLogID() (string, error) {
	return identity.NewID()
}
