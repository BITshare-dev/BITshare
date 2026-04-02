package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

type PublicDownloadRepository struct {
	db *gorm.DB
}

type ManagedFolderNode struct {
	ID         string
	ParentID   *string
	Name       string
	SourcePath *string
}

func NewPublicDownloadRepository(db *gorm.DB) *PublicDownloadRepository {
	return &PublicDownloadRepository{db: db}
}

func (r *PublicDownloadRepository) FindManagedFileByID(ctx context.Context, fileID string) (*model.File, error) {
	var file model.File
	err := r.db.WithContext(ctx).
		Where("id = ?", fileID).
		Take(&file).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find managed file by id: %w", err)
	}

	return &file, nil
}

func (r *PublicDownloadRepository) FindManagedFolderByID(ctx context.Context, folderID string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("id = ?", folderID).
		Take(&folder).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find managed folder by id: %w", err)
	}

	return &folder, nil
}

func (r *PublicDownloadRepository) ListManagedFoldersByIDs(ctx context.Context, folderIDs []string) ([]ManagedFolderNode, error) {
	if len(folderIDs) == 0 {
		return nil, nil
	}

	var rows []ManagedFolderNode
	err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Select("id, parent_id, name, source_path").
		Where("id IN ?", folderIDs).
		Find(&rows).
		Error
	if err != nil {
		return nil, fmt.Errorf("list managed folders by ids: %w", err)
	}

	return rows, nil
}

func (r *PublicDownloadRepository) IncrementDownloadCount(ctx context.Context, fileID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var file model.File
		if err := tx.Select("id, folder_id").
			Where("id = ?", fileID).
			Take(&file).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.File{}).
			Where("id = ?", fileID).
			UpdateColumn("download_count", gorm.Expr("download_count + 1")).
			Error; err != nil {
			return err
		}

		eventID, err := identity.NewID()
		if err != nil {
			return fmt.Errorf("generate download event id: %w", err)
		}

		if err := tx.Create(&model.DownloadEvent{
			ID:        eventID,
			FileID:    fileID,
			CreatedAt: time.Now().UTC(),
		}).Error; err != nil {
			return err
		}

		return model.AdjustFolderStatsTx(tx, file.FolderID, 0, 1, 0)
	})
}

func (r *PublicDownloadRepository) IncrementDownloadCounts(ctx context.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var files []model.File
		if err := tx.Model(&model.File{}).
			Select("id, folder_id").
			Where("id IN ?", fileIDs).
			Find(&files).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.File{}).
			Where("id IN ?", fileIDs).
			UpdateColumn("download_count", gorm.Expr("download_count + 1")).
			Error; err != nil {
			return err
		}

		now := time.Now().UTC()
		events := make([]model.DownloadEvent, 0, len(fileIDs))
		for _, fileID := range fileIDs {
			eventID, err := identity.NewID()
			if err != nil {
				return fmt.Errorf("generate download event id: %w", err)
			}
			events = append(events, model.DownloadEvent{
				ID:        eventID,
				FileID:    fileID,
				CreatedAt: now,
			})
		}

		if err := tx.Create(&events).Error; err != nil {
			return err
		}

		downloadDeltaByFolder := make(map[string]int64)
		for _, file := range files {
			if file.FolderID == nil || strings.TrimSpace(*file.FolderID) == "" {
				continue
			}
			downloadDeltaByFolder[strings.TrimSpace(*file.FolderID)]++
		}

		for folderID, delta := range downloadDeltaByFolder {
			id := folderID
			if err := model.AdjustFolderStatsTx(tx, &id, 0, delta, 0); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *PublicDownloadRepository) ListManagedFilesByIDs(ctx context.Context, fileIDs []string) ([]model.File, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}

	var files []model.File
	err := r.db.WithContext(ctx).
		Where("id IN ?", fileIDs).
		Find(&files).Error
	if err != nil {
		return nil, fmt.Errorf("list managed files by ids: %w", err)
	}

	byID := make(map[string]model.File, len(files))
	for _, file := range files {
		byID[file.ID] = file
	}

	ordered := make([]model.File, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if file, ok := byID[strings.TrimSpace(fileID)]; ok {
			ordered = append(ordered, file)
		}
	}
	return ordered, nil
}

func (r *PublicDownloadRepository) ListManagedFoldersByParentIDs(ctx context.Context, parentIDs []string) ([]ManagedFolderNode, error) {
	if len(parentIDs) == 0 {
		return nil, nil
	}

	var rows []ManagedFolderNode
	err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Select("id, parent_id, name, source_path").
		Where("parent_id IN ?", parentIDs).
		Order("name ASC").
		Find(&rows).
		Error
	if err != nil {
		return nil, fmt.Errorf("list managed folders by parent ids: %w", err)
	}

	return rows, nil
}

func (r *PublicDownloadRepository) ListManagedFilesByFolderIDs(ctx context.Context, folderIDs []string) ([]model.File, error) {
	if len(folderIDs) == 0 {
		return nil, nil
	}

	var files []model.File
	err := r.db.WithContext(ctx).
		Where("folder_id IN ?", folderIDs).
		Order("created_at ASC").
		Find(&files).
		Error
	if err != nil {
		return nil, fmt.Errorf("list managed files by folder ids: %w", err)
	}

	slices.SortFunc(files, func(a, b model.File) int {
		if a.FolderID != nil && b.FolderID != nil && *a.FolderID != *b.FolderID {
			return strings.Compare(*a.FolderID, *b.FolderID)
		}
		return strings.Compare(a.Name, b.Name)
	})

	return files, nil
}
