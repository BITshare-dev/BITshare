package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func (r *ImportRepository) FindFolderBySourcePath(ctx context.Context, sourcePath string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).Where("source_path = ?", sourcePath).Take(&folder).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find folder by source path: %w", err)
	}
	return &folder, nil
}

func (r *ImportRepository) FindFileBySourcePath(ctx context.Context, sourcePath string) (*model.File, error) {
	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	fileName := filepath.Base(sourcePath)
	folderPath := filepath.Dir(sourcePath)

	var file model.File
	err := r.db.WithContext(ctx).
		Model(&model.File{}).
		Joins("JOIN folders ON folders.id = files.folder_id").
		Where("files.name = ?", fileName).
		Where("folders.source_path = ?", folderPath).
		Take(&file).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find file by source path: %w", err)
	}
	return &file, nil
}

func (r *ImportRepository) FolderNameExists(ctx context.Context, parentID *string, name string) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.Folder{}).Where("name = ?", name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check folder name conflict: %w", err)
	}
	return count > 0, nil
}

func (r *ImportRepository) FileNameExists(ctx context.Context, folderID *string, name string) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.File{}).Where("name = ?", name)
	if folderID == nil {
		query = query.Where("folder_id IS NULL")
	} else {
		query = query.Where("folder_id = ?", *folderID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check file name conflict: %w", err)
	}
	return count > 0, nil
}

func (r *ImportRepository) CreateFolder(ctx context.Context, folder *model.Folder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

func (r *ImportRepository) CreateFile(ctx context.Context, file *model.File) error {
	return r.db.WithContext(ctx).Create(file).Error
}
