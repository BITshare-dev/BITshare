package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

type uploadNamespaceEntryKind int

const (
	uploadNamespaceEntryFile uploadNamespaceEntryKind = iota
	uploadNamespaceEntryDirectory
)

func (s *PublicUploadService) resolveUploadFileNames(
	ctx context.Context,
	rootFolder *model.Folder,
	rootFolderDisplayPath string,
	files []normalizedUploadFile,
) error {
	if rootFolder == nil || rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return ErrUploadFolderNotFound
	}

	pendingPaths, err := s.repository.ListPendingRelativePathsByRootFolderID(ctx, rootFolder.ID)
	if err != nil {
		return fmt.Errorf("list pending upload paths: %w", err)
	}

	existingEntriesByDir := make(map[string]map[string]uploadNamespaceEntryKind)
	for _, pendingPath := range pendingPaths {
		relativeToRoot := repository.NormalizeStoredSubmissionRelativePath(rootFolderDisplayPath, pendingPath)
		if relativeToRoot == "" {
			continue
		}
		registerExistingUploadPath(existingEntriesByDir, relativeToRoot)
	}

	plannedEntriesByDir := make(map[string]map[string]uploadNamespaceEntryKind)
	for index := range files {
		entry := &files[index]
		if conflict := registerPlannedUploadPath(plannedEntriesByDir, entry.RelativeDir, entry.Name); conflict {
			return ErrUploadNameConflict
		}
		entry.RelativePath = buildRelativePath(entry.RelativeDir, entry.Name)
	}

	for _, relativeDir := range orderedUploadNamespaceDirs(plannedEntriesByDir) {
		if err := s.seedReservedNamesFromDisk(existingEntriesByDir, *rootFolder.SourcePath, relativeDir); err != nil {
			return err
		}
		existingEntries := ensureUploadNamespaceDir(existingEntriesByDir, relativeDir)
		plannedEntries := plannedEntriesByDir[relativeDir]
		for name := range plannedEntries {
			if _, exists := existingEntries[name]; exists {
				return ErrUploadNameConflict
			}
		}
	}

	return nil
}

func (s *PublicUploadService) seedReservedNamesFromDisk(
	reservedByDir map[string]map[string]uploadNamespaceEntryKind,
	rootSourcePath string,
	relativeDir string,
) error {
	relativeDir = repository.NormalizeRelativePathForStorage(relativeDir)
	entriesByName := ensureUploadNamespaceDir(reservedByDir, relativeDir)
	targetDir := rootSourcePath
	if relativeDir != "" {
		targetDir = filepath.Join(rootSourcePath, filepath.FromSlash(relativeDir))
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect upload target directory: %w", err)
	}

	for _, entry := range entries {
		kind := uploadNamespaceEntryFile
		if entry.IsDir() {
			kind = uploadNamespaceEntryDirectory
		}
		key := entry.Name()
		if _, exists := entriesByName[key]; !exists {
			entriesByName[key] = kind
		}
	}

	return nil
}

func ensureUploadNamespaceDir(entriesByDir map[string]map[string]uploadNamespaceEntryKind, relativeDir string) map[string]uploadNamespaceEntryKind {
	relativeDir = repository.NormalizeRelativePathForStorage(relativeDir)
	if _, exists := entriesByDir[relativeDir]; !exists {
		entriesByDir[relativeDir] = make(map[string]uploadNamespaceEntryKind)
	}
	return entriesByDir[relativeDir]
}

func reserveFileName(reservedByDir map[string]map[string]uploadNamespaceEntryKind, relativeDir string, fileName string) {
	dirEntries := ensureUploadNamespaceDir(reservedByDir, relativeDir)
	dirEntries[fileName] = uploadNamespaceEntryFile
}

func reserveFolderName(reservedByDir map[string]map[string]uploadNamespaceEntryKind, relativeDir string, folderName string) {
	dirEntries := ensureUploadNamespaceDir(reservedByDir, relativeDir)
	key := folderName
	if existing, exists := dirEntries[key]; exists && existing == uploadNamespaceEntryFile {
		return
	}
	dirEntries[key] = uploadNamespaceEntryDirectory
}

func registerPlannedUploadPath(entriesByDir map[string]map[string]uploadNamespaceEntryKind, relativeDir string, fileName string) bool {
	relativeDir = repository.NormalizeRelativePathForStorage(relativeDir)
	parentDir := ""
	if relativeDir != "" {
		for _, segment := range strings.Split(relativeDir, "/") {
			if registerPlannedUploadEntry(entriesByDir, parentDir, segment, uploadNamespaceEntryDirectory) {
				return true
			}
			parentDir = buildRelativePath(parentDir, segment)
		}
	}
	return registerPlannedUploadEntry(entriesByDir, relativeDir, fileName, uploadNamespaceEntryFile)
}

func registerPlannedUploadEntry(entriesByDir map[string]map[string]uploadNamespaceEntryKind, relativeDir string, name string, kind uploadNamespaceEntryKind) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}

	dirEntries := ensureUploadNamespaceDir(entriesByDir, relativeDir)
	key := name
	existing, exists := dirEntries[key]
	if !exists {
		dirEntries[key] = kind
		return false
	}
	return !(existing == uploadNamespaceEntryDirectory && kind == uploadNamespaceEntryDirectory)
}

func registerExistingUploadPath(entriesByDir map[string]map[string]uploadNamespaceEntryKind, relativePath string) {
	relativePath = repository.NormalizeRelativePathForStorage(relativePath)
	if relativePath == "" {
		return
	}

	relativeDir := repository.NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Dir(relativePath)))
	parentDir := ""
	if relativeDir != "" {
		for _, segment := range strings.Split(relativeDir, "/") {
			reserveFolderName(entriesByDir, parentDir, segment)
			parentDir = buildRelativePath(parentDir, segment)
		}
	}
	reserveFileName(entriesByDir, relativeDir, filepath.Base(relativePath))
}

func orderedUploadNamespaceDirs(entriesByDir map[string]map[string]uploadNamespaceEntryKind) []string {
	dirs := make([]string, 0, len(entriesByDir))
	for relativeDir := range entriesByDir {
		dirs = append(dirs, relativeDir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		leftDepth := uploadNamespaceDirDepth(dirs[i])
		rightDepth := uploadNamespaceDirDepth(dirs[j])
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return dirs[i] < dirs[j]
	})
	return dirs
}

func uploadNamespaceDirDepth(relativeDir string) int {
	relativeDir = repository.NormalizeRelativePathForStorage(relativeDir)
	if relativeDir == "" {
		return 0
	}
	return strings.Count(relativeDir, "/") + 1
}

func buildRelativePath(relativeDir string, fileName string) string {
	relativeDir = repository.NormalizeRelativePathForStorage(relativeDir)
	fileName = repository.NormalizeRelativePathForStorage(fileName)
	if relativeDir == "" {
		return fileName
	}
	return relativeDir + "/" + fileName
}

func isIgnoredUploadFile(originalName string, relativePath string) bool {
	name := strings.TrimSpace(filepath.Base(originalName))
	if name == "" && strings.TrimSpace(relativePath) != "" {
		name = strings.TrimSpace(filepath.Base(filepath.ToSlash(relativePath)))
	}
	return strings.EqualFold(name, ".DS_Store")
}
