package service

import (
	"context"
	"fmt"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

func (s *ImportService) ensureFolder(ctx context.Context, parentID *string, name string, sourcePath string, now time.Time) (*model.Folder, bool, string, error) {
	existing, err := s.repository.FindFolderBySourcePath(ctx, sourcePath)
	if err != nil {
		return nil, false, "", fmt.Errorf("find imported folder: %w", err)
	}
	if existing != nil {
		return existing, false, "", nil
	}

	conflict, err := s.repository.FolderNameExists(ctx, parentID, name)
	if err != nil {
		return nil, false, "", err
	}
	if conflict {
		return nil, false, fmt.Sprintf("folder name conflict: %s", sourcePath), nil
	}

	id, err := identity.NewID()
	if err != nil {
		return nil, false, "", fmt.Errorf("generate folder id: %w", err)
	}

	sourcePathCopy := sourcePath
	folder := &model.Folder{
		ID:         id,
		ParentID:   parentID,
		SourcePath: &sourcePathCopy,
		Name:       name,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repository.CreateFolder(ctx, folder); err != nil {
		return nil, false, "", fmt.Errorf("create folder: %w", err)
	}

	return folder, true, "", nil
}

func (s *ImportService) ensureFile(ctx context.Context, folderID *string, entry storage.ScannedEntry, now time.Time) (bool, string, error) {
	existing, err := s.repository.FindFileBySourcePath(ctx, entry.AbsolutePath)
	if err != nil {
		return false, "", fmt.Errorf("find imported file: %w", err)
	}
	if existing != nil {
		return false, "", nil
	}

	conflict, err := s.repository.FileNameExists(ctx, folderID, entry.Name)
	if err != nil {
		return false, "", err
	}
	if conflict {
		return false, fmt.Sprintf("file name conflict: %s", entry.AbsolutePath), nil
	}

	id, err := identity.NewID()
	if err != nil {
		return false, "", fmt.Errorf("generate file id: %w", err)
	}

	file := &model.File{
		ID:            id,
		FolderID:      folderID,
		Name:          entry.Name,
		Description:   "",
		Extension:     entry.Extension,
		MimeType:      entry.MimeType,
		Size:          entry.Size,
		DownloadCount: 0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repository.CreateFile(ctx, file); err != nil {
		return false, "", fmt.Errorf("create imported file: %w", err)
	}

	return true, "", nil
}

func (s *ImportService) validateNewManagedRoot(ctx context.Context, rootPath string) error {
	roots, err := s.repository.ListManagedRoots(ctx)
	if err != nil {
		return fmt.Errorf("list managed roots: %w", err)
	}

	candidate := normalizeComparableManagedPath(rootPath)
	for _, root := range roots {
		existingPath := normalizeOptionalPath(root.SourcePath)
		if existingPath == "" {
			continue
		}
		existingComparable := normalizeComparableManagedPath(existingPath)

		switch {
		case candidate == existingComparable:
			return &ManagedDirectoryConflictError{Message: "该目录已托管，请使用“重新扫描”。"}
		case isManagedPathWithin(candidate, existingComparable):
			return &ManagedDirectoryConflictError{Message: "该目录位于已托管目录内，请对上级托管目录执行“重新扫描”。"}
		case isManagedPathWithin(existingComparable, candidate):
			return &ManagedDirectoryConflictError{Message: "该目录包含已托管目录，不能重复导入父目录。"}
		}
	}

	return nil
}
