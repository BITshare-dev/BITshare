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

func EnsureManagedFolderPathTx(tx *gorm.DB, rootFolder *model.Folder, relativePath string, now time.Time) (*model.Folder, error) {
	current := rootFolder
	normalized := NormalizeRelativePathForStorage(relativePath)
	if normalized == "" {
		return current, nil
	}

	for _, segment := range strings.Split(normalized, "/") {
		var child model.Folder
		err := tx.
			Where("parent_id = ? AND name = ?", current.ID, segment).
			Take(&child).
			Error
		if err == nil {
			current = &child
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("find child folder: %w", err)
		}

		id, idErr := identity.NewID()
		if idErr != nil {
			return nil, fmt.Errorf("generate folder id: %w", idErr)
		}
		sourcePath := filepath.Join(strings.TrimSpace(derefString(current.SourcePath)), segment)
		child = model.Folder{
			ID:          id,
			ParentID:    &current.ID,
			SourcePath:  stringPtr(sourcePath),
			Name:        segment,
			Description: "",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tx.Create(&child).Error; err != nil {
			return nil, fmt.Errorf("create child folder: %w", err)
		}
		current = &child
	}

	return current, nil
}

func BuildFolderDisplayPath(ctx context.Context, db *gorm.DB, folderID *string) (string, error) {
	if db == nil || folderID == nil || strings.TrimSpace(*folderID) == "" {
		return "", nil
	}

	segments := make([]string, 0)
	currentID := folderID
	for currentID != nil && strings.TrimSpace(*currentID) != "" {
		var folder model.Folder
		if err := db.WithContext(ctx).
			Model(&model.Folder{}).
			Select("id, parent_id, name").
			Where("id = ?", *currentID).
			Take(&folder).
			Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			return "", fmt.Errorf("load folder path segment: %w", err)
		}
		segments = append(segments, NormalizeRelativePathForStorage(folder.Name))
		currentID = folder.ParentID
	}

	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}

	filtered := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		filtered = append(filtered, segment)
	}
	return strings.Join(filtered, "/"), nil
}

func BuildFolderDisplayPathFromFolder(ctx context.Context, db *gorm.DB, folder *model.Folder) (string, error) {
	if folder == nil {
		return "", nil
	}

	currentID := folder.ID
	return BuildFolderDisplayPath(ctx, db, &currentID)
}

func NormalizeStoredSubmissionRelativePath(rootDisplayPath string, relativePath string) string {
	rootDisplayPath = NormalizeRelativePathForStorage(rootDisplayPath)
	relativePath = NormalizeRelativePathForStorage(relativePath)

	if rootDisplayPath == "" {
		return relativePath
	}
	if relativePath == rootDisplayPath {
		return ""
	}

	prefix := rootDisplayPath + "/"
	if strings.HasPrefix(relativePath, prefix) {
		return strings.TrimPrefix(relativePath, prefix)
	}
	return relativePath
}

func BuildSubmissionDisplayPath(rootDisplayPath string, relativePath string) string {
	rootDisplayPath = NormalizeRelativePathForStorage(rootDisplayPath)
	relativePath = NormalizeStoredSubmissionRelativePath(rootDisplayPath, relativePath)

	switch {
	case rootDisplayPath == "":
		return relativePath
	case relativePath == "":
		return rootDisplayPath
	default:
		return rootDisplayPath + "/" + relativePath
	}
}

func NormalizeRelativePathForStorage(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.Trim(value, "/")
	if value == "" || value == "." {
		return ""
	}
	parts := strings.Split(value, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/")
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	copied := value
	return &copied
}
