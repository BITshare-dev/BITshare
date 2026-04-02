package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

func (s *ResourceManagementService) DeleteFile(ctx context.Context, fileID string, operatorID string, operatorIP string) error {
	current, err := s.repo.FindFileByID(ctx, strings.TrimSpace(fileID))
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}

	folder, err := s.repo.FindFolderByID(ctx, modelValue(current.FolderID))
	if err != nil {
		return err
	}
	filePath := model.BuildManagedFilePath(folderSourcePath(folder), current.Name)
	if filePath == "" {
		return ErrManagedFileNotFound
	}

	_, err = s.storage.MoveManagedFileToTrash(filePath)
	if err != nil {
		return fmt.Errorf("move managed file to trash: %w", err)
	}

	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate resource delete log id: %w", err)
	}
	now := s.nowFunc()
	if err := s.repo.DeleteFileWithLog(ctx, current.ID, operatorID, operatorIP, logID, current.Name, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFileNotFound
		}
		return fmt.Errorf("delete managed file: %w", err)
	}
	return nil
}

func (s *ResourceManagementService) DeleteFolder(ctx context.Context, folderID string, operatorID string, operatorIP string) error {
	current, err := s.repo.FindFolderByID(ctx, strings.TrimSpace(folderID))
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFolderNotFound
	}

	folderIDs, err := s.repo.ListFolderTreeIDs(ctx, current.ID)
	if err != nil {
		return err
	}
	if len(folderIDs) == 0 {
		return ErrManagedFolderNotFound
	}

	newPath := ""
	if current.SourcePath != nil && strings.TrimSpace(*current.SourcePath) != "" {
		newPath, err = s.storage.MoveManagedDirectoryToTrash(*current.SourcePath)
		if err != nil {
			return fmt.Errorf("move managed folder to trash: %w", err)
		}
	}

	now := s.nowFunc()
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate folder delete log id: %w", err)
	}
	if err := s.repo.DeleteFolderTreeWithLog(ctx, current.ID, folderIDs, newPath, operatorID, operatorIP, logID, current.Name, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFolderNotFound
		}
		return fmt.Errorf("delete managed folder: %w", err)
	}
	return nil
}
