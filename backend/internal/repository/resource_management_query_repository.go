package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type ManagedFileRow struct {
	ID            string
	Name          string
	Description   string
	Extension     string
	Size          int64
	DownloadCount int64
	FolderName    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ManagedFolderPathRow struct {
	ID         string
	ParentID   *string
	Name       string
	SourcePath *string
}

type ManagedFilePathRow struct {
	ID       string
	FolderID *string
	Name     string
}

type FolderTreeNode struct {
	ID string
}

func (r *ResourceManagementRepository) ListFiles(ctx context.Context, query string) ([]ManagedFileRow, error) {
	dbq := r.db.WithContext(ctx).
		Table("files").
		Select(`
			files.id,
			files.name,
			files.description,
			files.extension,
			files.size,
			files.download_count,
			files.created_at,
			files.updated_at,
			COALESCE(folders.name, '') AS folder_name
		`).
		Joins("LEFT JOIN folders ON folders.id = files.folder_id")
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		like := "%" + trimmed + "%"
		dbq = dbq.Where("files.name LIKE ? OR files.description LIKE ?", like, like)
	}

	var rows []ManagedFileRow
	if err := dbq.Order("files.updated_at DESC, files.id DESC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list managed files: %w", err)
	}
	return rows, nil
}

func (r *ResourceManagementRepository) FindFileByID(ctx context.Context, fileID string) (*model.File, error) {
	var file model.File
	err := r.db.WithContext(ctx).Where("id = ?", fileID).Take(&file).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find file: %w", err)
	}
	return &file, nil
}

func (r *ResourceManagementRepository) FindFolderByID(ctx context.Context, folderID string) (*model.Folder, error) {
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

func (r *ResourceManagementRepository) ListFolderTreeIDs(ctx context.Context, folderID string) ([]string, error) {
	var rows []FolderTreeNode
	query := `
		WITH RECURSIVE folder_tree(id) AS (
			SELECT id FROM folders WHERE id = ?
			UNION ALL
			SELECT folders.id
			FROM folders
			JOIN folder_tree ON folders.parent_id = folder_tree.id
		)
		SELECT id FROM folder_tree
	`
	if err := r.db.WithContext(ctx).Raw(query, folderID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list folder tree ids: %w", err)
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.ID)
	}
	return result, nil
}

func (r *ResourceManagementRepository) FolderNameExists(ctx context.Context, parentID *string, name, excludeFolderID string) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.Folder{}).
		Where("name = ?", name).
		Where("id <> ?", excludeFolderID)
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

func (r *ResourceManagementRepository) FileNameExists(ctx context.Context, folderID *string, name, excludeFileID string) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.File{}).
		Where("name = ?", name).
		Where("id <> ?", excludeFileID)
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

func (r *ResourceManagementRepository) ListFolderPaths(ctx context.Context) ([]ManagedFolderPathRow, error) {
	var rows []ManagedFolderPathRow
	if err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Select("id, parent_id, name, source_path").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list folder paths: %w", err)
	}
	return rows, nil
}

func (r *ResourceManagementRepository) ListFilePaths(ctx context.Context) ([]ManagedFilePathRow, error) {
	var rows []ManagedFilePathRow
	if err := r.db.WithContext(ctx).
		Model(&model.File{}).
		Select("id, folder_id, name").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list file paths: %w", err)
	}
	return rows, nil
}
