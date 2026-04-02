package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func (r *ResourceManagementRepository) UpdateFileMetadata(
	ctx context.Context,
	fileID string,
	name string,
	extension string,
	description string,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.File{}).Where("id = ?", fileID).Updates(map[string]any{
			"name":          name,
			"extension":     extension,
			"description":   description,
			"updated_at":    now,
		})
		if result.Error != nil {
			return fmt.Errorf("update file metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return createOperationLogTx(tx, logID, operatorID, "resource_updated", "file", fileID, name, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) UpdateFolderMetadata(
	ctx context.Context,
	folderID string,
	name string,
	description string,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Folder{}).Where("id = ?", folderID).Updates(map[string]any{
			"name":        name,
			"description": description,
			"updated_at":  now,
		})
		if result.Error != nil {
			return fmt.Errorf("update folder metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return createOperationLogTx(tx, logID, operatorID, "folder_updated", "folder", folderID, name, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) UpdateFolderTreePaths(
	ctx context.Context,
	folderID string,
	name string,
	description string,
	folderSourcePaths map[string]string,
	operatorID string,
	operatorIP string,
	logID string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rootUpdates := map[string]any{
			"name":        name,
			"description": description,
			"updated_at":  now,
		}
		if sourcePath, ok := folderSourcePaths[folderID]; ok {
			rootUpdates["source_path"] = sourcePath
		}

		result := tx.Model(&model.Folder{}).Where("id = ?", folderID).Updates(rootUpdates)
		if result.Error != nil {
			return fmt.Errorf("update root folder metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		for id, sourcePath := range folderSourcePaths {
			if id == folderID {
				continue
			}
			if err := tx.Model(&model.Folder{}).Where("id = ?", id).Updates(map[string]any{
				"source_path": sourcePath,
				"updated_at":  now,
			}).Error; err != nil {
				return fmt.Errorf("update child folder path: %w", err)
			}
		}

		return createOperationLogTx(tx, logID, operatorID, "folder_updated", "folder", folderID, name, operatorIP, now)
	})
}
