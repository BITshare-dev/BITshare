package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

var (
	ErrInvalidImportPath   = errors.New("invalid import path")
	ErrFolderTreeNotFound  = errors.New("folder not found")
	ErrManagedRootRequired = errors.New("managed root folder required")
)

type ManagedDirectoryConflictError struct {
	Message string
}

func (e *ManagedDirectoryConflictError) Error() string {
	if e == nil {
		return "managed directory conflict"
	}
	return e.Message
}

type ManagedDirectoryUnavailableError struct {
	Path string
}

func (e *ManagedDirectoryUnavailableError) Error() string {
	if e == nil || strings.TrimSpace(e.Path) == "" {
		return "managed directory path is unavailable"
	}
	return fmt.Sprintf("托管目录不可用：%s", e.Path)
}

type ImportService struct {
	repository *repository.ImportRepository
	storage    *storage.Service
	nowFunc    func() time.Time
}

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

type ManagedDirectoryRescanResult struct {
	RootPath       string `json:"root_path"`
	AddedFolders   int    `json:"added_folders"`
	AddedFiles     int    `json:"added_files"`
	UpdatedFolders int    `json:"updated_folders"`
	UpdatedFiles   int    `json:"updated_files"`
	DeletedFolders int    `json:"deleted_folders"`
	DeletedFiles   int    `json:"deleted_files"`
}

type FolderTreeNode struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	SourcePath string               `json:"source_path"`
	Status     model.ResourceStatus `json:"status"`
	Folders    []FolderTreeNode     `json:"folders"`
	Files      []FolderTreeFile     `json:"files"`
}

type FolderTreeFile struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	OriginalName  string               `json:"original_name"`
	Status        model.ResourceStatus `json:"status"`
	Size          int64                `json:"size"`
	DownloadCount int64                `json:"download_count"`
}

type ImportDirectoryItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ImportDirectoryBrowseResult struct {
	CurrentPath string                `json:"current_path"`
	ParentPath  string                `json:"parent_path"`
	Items       []ImportDirectoryItem `json:"items"`
}

func NewImportService(repository *repository.ImportRepository, storageService *storage.Service) *ImportService {
	return &ImportService{
		repository: repository,
		storage:    storageService,
		nowFunc:    func() time.Time { return time.Now().UTC() },
	}
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
	for _, folder := range folders {
		if folder.SourcePath == nil || strings.TrimSpace(*folder.SourcePath) == "" {
			continue
		}
		existingFoldersByPath[normalizeRescanPath(*folder.SourcePath)] = folder
	}

	existingFilesBySourcePath := make(map[string]repository.ManagedSubtreeFileRow, len(files))
	existingFilesByDiskPath := make(map[string]repository.ManagedSubtreeFileRow, len(files))
	for _, file := range files {
		if file.SourcePath != nil && strings.TrimSpace(*file.SourcePath) != "" {
			existingFilesBySourcePath[normalizeRescanPath(*file.SourcePath)] = file
		}
		if strings.TrimSpace(file.DiskPath) != "" {
			existingFilesByDiskPath[normalizeRescanPath(file.DiskPath)] = file
		}
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
				normalizeOptionalPath(existing.SourcePath) == normalizedPath &&
				existing.Status == model.ResourceStatusActive {
				continue
			}
			if optionalStringValue(existing.ParentID) == optionalStringValue(parentID) &&
				existing.Name == name &&
				normalizeOptionalPath(existing.SourcePath) == normalizedPath &&
				existing.Status == model.ResourceStatusActive {
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
			Status:      model.ResourceStatusActive,
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
		title := strings.TrimSuffix(entry.Name, filepath.Ext(entry.Name))
		if title == "" {
			title = entry.Name
		}

		if existing, ok := matchManagedFileByPath(filePath, existingFilesBySourcePath, existingFilesByDiskPath, matchedFileIDs); ok {
			matchedFileIDs[existing.ID] = struct{}{}
			description := existing.Description
			if optionalStringValue(existing.FolderID) == optionalStringValue(folderID) &&
				normalizeOptionalPath(existing.SourcePath) == normalizeRescanPath(filePath) &&
				normalizeRescanPath(existing.DiskPath) == normalizeRescanPath(filePath) &&
				existing.Title == title &&
				existing.OriginalName == entry.Name &&
				existing.StoredName == entry.Name &&
				existing.Extension == entry.Extension &&
				existing.MimeType == entry.MimeType &&
				existing.Size == entry.Size &&
				existing.Status == model.ResourceStatusActive {
				continue
			}
			updatedFiles = append(updatedFiles, repository.ManagedFileUpdate{
				ID:           existing.ID,
				FolderID:     folderID,
				SourcePath:   normalizeRescanPath(filePath),
				DiskPath:     normalizeRescanPath(filePath),
				Title:        title,
				Description:  description,
				OriginalName: entry.Name,
				StoredName:   entry.Name,
				Extension:    entry.Extension,
				MimeType:     entry.MimeType,
				Size:         entry.Size,
			})
			result.UpdatedFiles++
			continue
		}

		id, err := identity.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate rescanned file id: %w", err)
		}
		sourcePath := normalizeRescanPath(filePath)
		addedFiles = append(addedFiles, &model.File{
			ID:            id,
			FolderID:      folderID,
			SubmissionID:  nil,
			SourcePath:    &sourcePath,
			Title:         title,
			Description:   "",
			OriginalName:  entry.Name,
			StoredName:    entry.Name,
			Extension:     entry.Extension,
			MimeType:      entry.MimeType,
			Size:          entry.Size,
			DiskPath:      sourcePath,
			Status:        model.ResourceStatusActive,
			DownloadCount: 0,
			UploaderIP:    "",
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
			Status:     folder.Status,
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
			Title:         file.Title,
			OriginalName:  file.OriginalName,
			Status:        file.Status,
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
		Status:     model.ResourceStatusActive,
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
	sourcePathCopy := entry.AbsolutePath
	file := &model.File{
		ID:            id,
		FolderID:      folderID,
		SubmissionID:  nil,
		SourcePath:    &sourcePathCopy,
		Title:         strings.TrimSuffix(entry.Name, filepath.Ext(entry.Name)),
		OriginalName:  entry.Name,
		StoredName:    entry.Name,
		Extension:     entry.Extension,
		MimeType:      entry.MimeType,
		Size:          entry.Size,
		DiskPath:      entry.AbsolutePath,
		Status:        model.ResourceStatusActive,
		DownloadCount: 0,
		UploaderIP:    "",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repository.CreateFile(ctx, file); err != nil {
		return false, "", fmt.Errorf("create imported file: %w", err)
	}
	return true, "", nil
}

func shouldSkipEntry(relativePath string, skippedPrefixes map[string]struct{}) bool {
	for prefix := range skippedPrefixes {
		if relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/") {
			return true
		}
	}
	return false
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
	bySourcePath map[string]repository.ManagedSubtreeFileRow,
	byDiskPath map[string]repository.ManagedSubtreeFileRow,
	matched map[string]struct{},
) (repository.ManagedSubtreeFileRow, bool) {
	normalizedPath := normalizeRescanPath(path)
	if file, ok := bySourcePath[normalizedPath]; ok {
		if _, alreadyMatched := matched[file.ID]; !alreadyMatched {
			return file, true
		}
	}
	if file, ok := byDiskPath[normalizedPath]; ok {
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
