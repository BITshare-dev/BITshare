package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
)

func shouldSkipEntry(relativePath string, skippedPrefixes map[string]struct{}) bool {
	for prefix := range skippedPrefixes {
		if relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/") {
			return true
		}
	}
	return false
}

func buildFilesystemSnapshot(rootPath string, entries []storage.ScannedEntry) ([]string, map[string]storage.ScannedEntry) {
	folderPaths := []string{normalizeRescanPath(rootPath)}
	files := make(map[string]storage.ScannedEntry)

	for _, entry := range entries {
		absolutePath := normalizeRescanPath(entry.AbsolutePath)
		if entry.IsDir {
			folderPaths = append(folderPaths, absolutePath)
			continue
		}
		files[absolutePath] = entry
	}

	sort.Slice(folderPaths, func(i, j int) bool {
		leftDepth := strings.Count(folderPaths[i], string(filepath.Separator))
		rightDepth := strings.Count(folderPaths[j], string(filepath.Separator))
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return folderPaths[i] < folderPaths[j]
	})

	return folderPaths, files
}

func matchManagedFileByPath(
	path string,
	byPath map[string]repository.ManagedSubtreeFileRow,
	matched map[string]struct{},
) (repository.ManagedSubtreeFileRow, bool) {
	normalizedPath := normalizeRescanPath(path)
	if file, ok := byPath[normalizedPath]; ok {
		if _, alreadyMatched := matched[file.ID]; !alreadyMatched {
			return file, true
		}
	}
	return repository.ManagedSubtreeFileRow{}, false
}

func normalizeComparableManagedPath(path string) string {
	cleaned := normalizeRescanPath(path)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err == nil && strings.TrimSpace(resolved) != "" {
		return normalizeRescanPath(resolved)
	}
	return cleaned
}

func normalizeRescanPath(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}

func normalizeOptionalPath(path *string) string {
	if path == nil {
		return ""
	}
	return normalizeRescanPath(*path)
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func isManagedPathWithin(path, root string) bool {
	path = normalizeRescanPath(path)
	root = normalizeRescanPath(root)
	if path == "" || root == "" || path == root {
		return false
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

func stringPtr(value string) *string {
	copied := value
	return &copied
}

func resolveBrowsePath(rootPath string) (string, error) {
	trimmed := strings.TrimSpace(rootPath)
	if trimmed == "" {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			return filepath.Clean(home), nil
		}
		return string(os.PathSeparator), nil
	}

	cleaned := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleaned) {
		return "", ErrInvalidImportPath
	}

	info, err := os.Stat(cleaned)
	if err != nil {
		return "", ErrInvalidImportPath
	}
	if !info.IsDir() {
		return "", ErrInvalidImportPath
	}

	return cleaned, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
