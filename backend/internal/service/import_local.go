package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"openshare/backend/internal/model"
)

type LocalImportInput struct {
	RootPath   string
	AdminID    string
	OperatorIP string
}

type LocalImportResult struct {
	RootPath        string   `json:"root_path"`
	ImportedFolders int      `json:"imported_folders"`
	ImportedFiles   int      `json:"imported_files"`
	SkippedFolders  int      `json:"skipped_folders"`
	SkippedFiles    int      `json:"skipped_files"`
	Conflicts       []string `json:"conflicts"`
}

func (s *ImportService) ImportLocalDirectory(ctx context.Context, input LocalImportInput) (*LocalImportResult, error) {
	rootPath := filepath.Clean(strings.TrimSpace(input.RootPath))
	if rootPath == "" || !filepath.IsAbs(rootPath) {
		return nil, ErrInvalidImportPath
	}

	entries, err := s.storage.ScanDirectory(rootPath)
	if err != nil {
		return nil, fmt.Errorf("scan local directory: %w", err)
	}
	if err := s.validateNewManagedRoot(ctx, rootPath); err != nil {
		return nil, err
	}

	now := s.nowFunc()
	result := &LocalImportResult{
		RootPath:  rootPath,
		Conflicts: make([]string, 0),
	}

	rootFolder, created, conflict, err := s.ensureFolder(ctx, nil, filepath.Base(rootPath), rootPath, now)
	if err != nil {
		return nil, err
	}
	if conflict != "" {
		result.Conflicts = append(result.Conflicts, conflict)
		return result, nil
	}
	if created {
		result.ImportedFolders++
	} else {
		result.SkippedFolders++
	}

	folderMap := map[string]*model.Folder{
		".": rootFolder,
		"":  rootFolder,
	}
	skippedPrefixes := make(map[string]struct{})

	for _, entry := range entries {
		if shouldSkipEntry(entry.RelativePath, skippedPrefixes) {
			if entry.IsDir {
				result.SkippedFolders++
			} else {
				result.SkippedFiles++
			}
			continue
		}

		parentRelative := filepath.ToSlash(filepath.Dir(entry.RelativePath))
		parentFolder, ok := folderMap[parentRelative]
		if parentRelative == "." || parentRelative == "" {
			parentFolder = rootFolder
			ok = true
		}
		if !ok {
			if entry.IsDir {
				skippedPrefixes[entry.RelativePath] = struct{}{}
				result.SkippedFolders++
			} else {
				result.SkippedFiles++
			}
			continue
		}

		if entry.IsDir {
			folder, created, conflict, err := s.ensureFolder(ctx, &parentFolder.ID, entry.Name, entry.AbsolutePath, now)
			if err != nil {
				return nil, err
			}
			if conflict != "" {
				result.Conflicts = append(result.Conflicts, conflict)
				skippedPrefixes[entry.RelativePath] = struct{}{}
				result.SkippedFolders++
				continue
			}
			folderMap[entry.RelativePath] = folder
			if created {
				result.ImportedFolders++
			} else {
				result.SkippedFolders++
			}
			continue
		}

		created, conflict, err := s.ensureFile(ctx, &parentFolder.ID, entry, now)
		if err != nil {
			return nil, err
		}
		if conflict != "" {
			result.Conflicts = append(result.Conflicts, conflict)
			result.SkippedFiles++
			continue
		}
		if created {
			result.ImportedFiles++
		} else {
			result.SkippedFiles++
		}
	}

	detail, _ := json.Marshal(result)
	_ = s.repository.LogOperation(ctx, input.AdminID, "local_import", "folder", rootFolder.ID, string(detail), input.OperatorIP, now)

	return result, nil
}
