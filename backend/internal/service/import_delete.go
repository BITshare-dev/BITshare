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

func (s *ImportService) UnmanageManagedDirectory(ctx context.Context, folderID, adminID, operatorIP string) error {
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
		detail = *folder.SourcePath
	}

	result, err := s.repository.UnmanageManagedRootWithLog(ctx, folder.ID, adminID, operatorIP, detail, logID, s.nowFunc())
	if err != nil {
		if errors.Is(err, repository.ErrManagedRootRequired) {
			return ErrManagedRootRequired
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrFolderTreeNotFound
		}
		return fmt.Errorf("unmanage managed directory: %w", err)
	}

	for _, stagingPath := range result.PendingStagingPaths {
		if err := s.storage.DeleteStagedFile(stagingPath); err != nil {
			return fmt.Errorf("cleanup pending staged file: %w", err)
		}
	}

	return nil
}
