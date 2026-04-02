package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

type UploadRepository struct {
	db *gorm.DB
}

type ApprovedUploadBatchItem struct {
	SubmissionID      string
	FinalName         string
	FinalRelativePath string
}

func NewUploadRepository(db *gorm.DB) *UploadRepository {
	return &UploadRepository{db: db}
}

func (r *UploadRepository) DB() *gorm.DB {
	return r.db
}

func (r *UploadRepository) FolderExists(ctx context.Context, folderID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Where("id = ?", folderID).
		Count(&count).
		Error
	if err != nil {
		return false, fmt.Errorf("check folder existence: %w", err)
	}

	return count > 0, nil
}

func (r *UploadRepository) FindManagedFolderByID(ctx context.Context, folderID string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("id = ?", folderID).
		Take(&folder).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find managed folder: %w", err)
	}
	return &folder, nil
}

func (r *UploadRepository) BuildFolderDisplayPath(ctx context.Context, folderID *string) (string, error) {
	return BuildFolderDisplayPath(ctx, r.db, folderID)
}

func (r *UploadRepository) ListPendingRelativePathsByRootFolderID(ctx context.Context, rootFolderID string) ([]string, error) {
	type pendingPathRow struct {
		RelativePath string `gorm:"column:relative_path"`
	}

	var rows []pendingPathRow
	if err := r.db.WithContext(ctx).
		Table("submissions").
		Select("submissions.relative_path AS relative_path").
		Where("submissions.folder_id = ?", rootFolderID).
		Where("submissions.status = ?", model.SubmissionStatusPending).
		Where("submissions.staging_path <> ''").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list pending relative paths: %w", err)
	}

	paths := make([]string, 0, len(rows))
	for _, row := range rows {
		paths = append(paths, row.RelativePath)
	}
	return paths, nil
}

func (r *UploadRepository) CreateUploadBatch(ctx context.Context, submissions []model.Submission) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range submissions {
			if err := tx.Create(&submissions[i]).Error; err != nil {
				return fmt.Errorf("create submission: %w", err)
			}
		}
		return nil
	})
}

func (r *UploadRepository) CreateApprovedUploadBatch(
	ctx context.Context,
	rootFolder *model.Folder,
	submissions []model.Submission,
	items []ApprovedUploadBatchItem,
	adminID string,
	operatorIP string,
	reviewedAt time.Time,
) error {
	if rootFolder == nil {
		return fmt.Errorf("root folder is required")
	}

	itemBySubmissionID := make(map[string]ApprovedUploadBatchItem, len(items))
	for _, item := range items {
		itemBySubmissionID[item.SubmissionID] = item
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range submissions {
			submission := submissions[i]
			item, ok := itemBySubmissionID[submission.ID]
			if !ok {
				return fmt.Errorf("approved upload item missing for submission %s", submission.ID)
			}

			targetFolder := rootFolder
			relativeDir := NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Dir(item.FinalRelativePath)))
			if relativeDir != "" {
				leaf, err := EnsureManagedFolderPathTx(tx, rootFolder, relativeDir, reviewedAt)
				if err != nil {
					return fmt.Errorf("ensure target folder path: %w", err)
				}
				targetFolder = leaf
			}

			fileID, err := identity.NewID()
			if err != nil {
				return fmt.Errorf("generate approved file id: %w", err)
			}
			file := &model.File{
				ID:            fileID,
				FolderID:      &targetFolder.ID,
				Name:          item.FinalName,
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

			submission.FileID = &fileID
			submission.Name = item.FinalName
			submission.RelativePath = item.FinalRelativePath
			submission.Status = model.SubmissionStatusApproved
			submission.ReviewReason = ""
			submission.StagingPath = ""
			submission.ReviewerID = &adminID
			submission.ReviewedAt = &reviewedAt
			submission.UpdatedAt = reviewedAt
			if err := tx.Create(&submission).Error; err != nil {
				return fmt.Errorf("create approved submission: %w", err)
			}

			logID, err := identity.NewID()
			if err != nil {
				return fmt.Errorf("generate operation log id: %w", err)
			}
			entry := &model.OperationLog{
				ID:         logID,
				AdminID:    &adminID,
				Action:     "submission_approved",
				TargetType: "submission",
				TargetID:   submission.ID,
				Detail:     item.FinalName,
				IP:         operatorIP,
				CreatedAt:  reviewedAt,
			}
			if err := tx.Create(entry).Error; err != nil {
				return fmt.Errorf("create operation log: %w", err)
			}
		}
		return nil
	})
}
