package service

import (
	"context"
	"fmt"
)

type FolderTreeNode struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	SourcePath string               `json:"source_path"`
	Folders    []FolderTreeNode     `json:"folders"`
	Files      []FolderTreeFile     `json:"files"`
}

type FolderTreeFile struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Size          int64                `json:"size"`
	DownloadCount int64                `json:"download_count"`
}

func (s *ImportService) GetFolderTree(ctx context.Context) ([]FolderTreeNode, error) {
	folders, err := s.repository.ListFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	files, err := s.repository.ListFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	nodes := make(map[string]*FolderTreeNode, len(folders))
	childrenByParent := make(map[string][]string)
	rootIDs := make([]string, 0)

	for _, folder := range folders {
		nodes[folder.ID] = &FolderTreeNode{
			ID:         folder.ID,
			Name:       folder.Name,
			SourcePath: derefString(folder.SourcePath),
			Folders:    []FolderTreeNode{},
			Files:      []FolderTreeFile{},
		}
	}
	for _, folder := range folders {
		if folder.ParentID == nil {
			rootIDs = append(rootIDs, folder.ID)
			continue
		}
		childrenByParent[*folder.ParentID] = append(childrenByParent[*folder.ParentID], folder.ID)
	}
	for _, file := range files {
		if file.FolderID == nil {
			continue
		}
		parent := nodes[*file.FolderID]
		if parent == nil {
			continue
		}
		parent.Files = append(parent.Files, FolderTreeFile{
			ID:            file.ID,
			Name:          file.Name,
			Size:          file.Size,
			DownloadCount: file.DownloadCount,
		})
	}

	var build func(string) FolderTreeNode
	build = func(folderID string) FolderTreeNode {
		node := nodes[folderID]
		result := *node
		result.Folders = make([]FolderTreeNode, 0, len(childrenByParent[folderID]))
		for _, childID := range childrenByParent[folderID] {
			result.Folders = append(result.Folders, build(childID))
		}
		return result
	}

	result := make([]FolderTreeNode, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		result = append(result, build(rootID))
	}

	return result, nil
}
