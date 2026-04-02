package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

func (s *ImportService) DeleteManagedDirectory(ctx context.Context, folderID, adminID, operatorIP string) error {
	folder, err := s.repository.FindFolderByID(ctx, strings.TrimSpace(folderID))
	if err != nil {
		return fmt.Errorf("find folder: %w", err)
	}
	if folder == nil {
		return ErrFolderTreeNotFound
	}
	if folder.ParentID != nil {
		return ErrManagedRootRequired
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate operation log id: %w", err)
	}

	detail := folder.Name
	if folder.SourcePath != nil && strings.TrimSpace(*folder.SourcePath) != "" {
		if _, err := s.storage.MoveManagedDirectoryToTrash(*folder.SourcePath); err != nil {
			return fmt.Errorf("move managed root to trash: %w", err)
		}
		detail = *folder.SourcePath
	}

	if err := s.repository.DeleteManagedRootWithLog(ctx, folder.ID, adminID, operatorIP, detail, logID, s.nowFunc()); err != nil {
		if errors.Is(err, repository.ErrManagedRootRequired) {
			return ErrManagedRootRequired
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrFolderTreeNotFound
		}
		return fmt.Errorf("delete managed directory: %w", err)
	}

	return nil
}
