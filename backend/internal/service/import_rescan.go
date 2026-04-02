package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

type ManagedDirectoryRescanResult struct {
	RootPath       string `json:"root_path"`
	AddedFolders   int    `json:"added_folders"`
	AddedFiles     int    `json:"added_files"`
	UpdatedFolders int    `json:"updated_folders"`
	UpdatedFiles   int    `json:"updated_files"`
	DeletedFolders int    `json:"deleted_folders"`
	DeletedFiles   int    `json:"deleted_files"`
}

func (s *ImportService) RescanManagedDirectory(ctx context.Context, folderID, adminID, operatorIP string) (*ManagedDirectoryRescanResult, error) {
	rootFolder, err := s.repository.FindFolderByID(ctx, strings.TrimSpace(folderID))
	if err != nil {
		return nil, fmt.Errorf("find managed root: %w", err)
	}
	if rootFolder == nil {
		return nil, ErrFolderTreeNotFound
	}
	if rootFolder.ParentID != nil {
		return nil, ErrManagedRootRequired
	}
	if rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, &ManagedDirectoryUnavailableError{}
	}

	rootPath := filepath.Clean(strings.TrimSpace(*rootFolder.SourcePath))
	info, err := os.Stat(rootPath)
	if err != nil || !info.IsDir() {
		return nil, &ManagedDirectoryUnavailableError{Path: rootPath}
	}

	entries, err := s.storage.ScanDirectory(rootPath)
	if err != nil {
		return nil, fmt.Errorf("scan managed directory: %w", err)
	}

	folders, err := s.repository.ListManagedSubtreeFolders(ctx, rootFolder.ID)
	if err != nil {
		return nil, fmt.Errorf("list managed subtree folders: %w", err)
	}
	files, err := s.repository.ListManagedSubtreeFiles(ctx, rootFolder.ID)
	if err != nil {
		return nil, fmt.Errorf("list managed subtree files: %w", err)
	}

	now := s.nowFunc()
	result := &ManagedDirectoryRescanResult{RootPath: rootPath}

	fsFolderPaths, fsFiles := buildFilesystemSnapshot(rootPath, entries)
	existingFoldersByPath := make(map[string]repository.ManagedSubtreeFolderRow, len(folders))
	folderSourcePathByID := make(map[string]string, len(folders))
	for _, folder := range folders {
		if folder.SourcePath == nil || strings.TrimSpace(*folder.SourcePath) == "" {
			continue
		}
		normalized := normalizeRescanPath(*folder.SourcePath)
		existingFoldersByPath[normalized] = folder
		folderSourcePathByID[folder.ID] = normalized
	}

	existingFilesByPath := make(map[string]repository.ManagedSubtreeFileRow, len(files))
	for _, file := range files {
		folderPath := folderSourcePathByID[optionalStringValue(file.FolderID)]
		if folderPath == "" || strings.TrimSpace(file.Name) == "" {
			continue
		}
		existingFilesByPath[normalizeRescanPath(filepath.Join(folderPath, file.Name))] = file
	}

	matchedFolderIDs := make(map[string]struct{}, len(folders))
	pathToFolderID := make(map[string]string, len(fsFolderPaths))
	updatedFolders := make([]repository.ManagedFolderUpdate, 0)
	addedFolders := make([]*model.Folder, 0)

	for _, folderPath := range fsFolderPaths {
		normalizedPath := normalizeRescanPath(folderPath)
		var parentID *string
		if normalizedPath != normalizeRescanPath(rootPath) {
			parentPath := normalizeRescanPath(filepath.Dir(normalizedPath))
			parentValue := pathToFolderID[parentPath]
			parentID = &parentValue
		}
		name := filepath.Base(normalizedPath)

		if existing, ok := existingFoldersByPath[normalizedPath]; ok {
			matchedFolderIDs[existing.ID] = struct{}{}
			pathToFolderID[normalizedPath] = existing.ID
			if existing.ParentID == nil && parentID == nil &&
				existing.Name == name &&
				normalizeOptionalPath(existing.SourcePath) == normalizedPath {
				continue
			}
			if optionalStringValue(existing.ParentID) == optionalStringValue(parentID) &&
				existing.Name == name &&
				normalizeOptionalPath(existing.SourcePath) == normalizedPath {
				continue
			}
			updatedFolders = append(updatedFolders, repository.ManagedFolderUpdate{
				ID:         existing.ID,
				ParentID:   parentID,
				Name:       name,
				SourcePath: normalizedPath,
			})
			result.UpdatedFolders++
			continue
		}

		id, err := identity.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate rescanned folder id: %w", err)
		}
		addedFolders = append(addedFolders, &model.Folder{
			ID:          id,
			ParentID:    parentID,
			SourcePath:  stringPtr(normalizedPath),
			Name:        name,
			Description: "",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		pathToFolderID[normalizedPath] = id
		result.AddedFolders++
	}

	matchedFileIDs := make(map[string]struct{}, len(files))
	updatedFiles := make([]repository.ManagedFileUpdate, 0)
	addedFiles := make([]*model.File, 0)

	filePaths := make([]string, 0, len(fsFiles))
	for path := range fsFiles {
		filePaths = append(filePaths, path)
	}
	sort.Strings(filePaths)

	for _, filePath := range filePaths {
		entry := fsFiles[filePath]
		parentPath := normalizeRescanPath(filepath.Dir(filePath))
		parentValue := pathToFolderID[parentPath]
		folderID := &parentValue

		if existing, ok := matchManagedFileByPath(filePath, existingFilesByPath, matchedFileIDs); ok {
			matchedFileIDs[existing.ID] = struct{}{}
			description := existing.Description
			if optionalStringValue(existing.FolderID) == optionalStringValue(folderID) &&
				existing.Name == entry.Name &&
				existing.Extension == entry.Extension &&
				existing.MimeType == entry.MimeType &&
				existing.Size == entry.Size {
				continue
			}
			updatedFiles = append(updatedFiles, repository.ManagedFileUpdate{
				ID:          existing.ID,
				FolderID:    folderID,
				Name:        entry.Name,
				Description: description,
				Extension:   entry.Extension,
				MimeType:    entry.MimeType,
				Size:        entry.Size,
			})
			result.UpdatedFiles++
			continue
		}

		id, err := identity.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate rescanned file id: %w", err)
		}
		addedFiles = append(addedFiles, &model.File{
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
		})
		result.AddedFiles++
	}

	deletedFileIDs := make([]string, 0)
	for _, file := range files {
		if _, ok := matchedFileIDs[file.ID]; ok {
			continue
		}
		deletedFileIDs = append(deletedFileIDs, file.ID)
	}
	result.DeletedFiles = len(deletedFileIDs)

	deletedFolderIDs := make([]string, 0)
	for _, folder := range folders {
		if folder.ID == rootFolder.ID {
			continue
		}
		if _, ok := matchedFolderIDs[folder.ID]; ok {
			continue
		}
		deletedFolderIDs = append(deletedFolderIDs, folder.ID)
	}
	result.DeletedFolders = len(deletedFolderIDs)

	detail, _ := json.Marshal(result)
	if err := s.repository.ApplyRescanSync(ctx, repository.RescanSyncInput{
		RootFolderID:     rootFolder.ID,
		OperatorID:       adminID,
		OperatorIP:       operatorIP,
		Detail:           string(detail),
		Now:              now,
		AddedFolders:     addedFolders,
		UpdatedFolders:   updatedFolders,
		DeletedFolderIDs: deletedFolderIDs,
		AddedFiles:       addedFiles,
		UpdatedFiles:     updatedFiles,
		DeletedFileIDs:   deletedFileIDs,
	}); err != nil {
		return nil, fmt.Errorf("apply managed directory rescan: %w", err)
	}

	return result, nil
}
