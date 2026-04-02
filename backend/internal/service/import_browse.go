package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ImportDirectoryItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ImportDirectoryBrowseResult struct {
	CurrentPath string                `json:"current_path"`
	ParentPath  string                `json:"parent_path"`
	Items       []ImportDirectoryItem `json:"items"`
}

func (s *ImportService) ListDirectories(_ context.Context, rootPath string) (*ImportDirectoryBrowseResult, error) {
	currentPath, err := resolveBrowsePath(rootPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil, fmt.Errorf("read import directory: %w", err)
	}

	items := make([]ImportDirectoryItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		childPath := filepath.Join(currentPath, entry.Name())
		items = append(items, ImportDirectoryItem{
			Name: entry.Name(),
			Path: childPath,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	parentPath := filepath.Dir(currentPath)
	if parentPath == "." || parentPath == currentPath {
		parentPath = ""
	}

	return &ImportDirectoryBrowseResult{
		CurrentPath: currentPath,
		ParentPath:  parentPath,
		Items:       items,
	}, nil
}
