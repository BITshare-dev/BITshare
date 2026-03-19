package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

var (
	ErrInvalidUploadInput   = errors.New("invalid upload input")
	ErrUploadReceiptExists  = errors.New("receipt code already exists")
	ErrUploadFileTooLarge   = errors.New("upload file too large")
	ErrUploadEmptyFile      = errors.New("upload file is empty")
	ErrInvalidFileExtension = errors.New("invalid file extension")
	ErrInvalidFileMIMEType  = errors.New("invalid file mime type")
	ErrReceiptCodeGenerate  = errors.New("failed to generate receipt code")
	ErrUploadFolderRequired = errors.New("upload target folder is required")
	ErrUploadFolderNotFound = errors.New("upload target folder not found")
)

const maxGeneratedReceiptAttempts = 5

type PublicUploadService struct {
	config        config.UploadConfig
	repository    *repository.UploadRepository
	receiptCodes  *ReceiptCodeService
	storage       *storage.Service
	systemSetting *SystemSettingService
	nowFunc       func() time.Time
}

type PublicUploadInput struct {
	Description   string
	ReceiptCode   string
	FolderID      string
	UploaderIP    string
	DirectPublish bool
	Files         []PublicUploadFileInput
}

type PublicUploadFileInput struct {
	OriginalName string
	RelativePath string
	DeclaredMIME string
	File         io.Reader
}

type PublicUploadResult struct {
	ReceiptCode string                 `json:"receipt_code"`
	Status      model.SubmissionStatus `json:"status"`
	ItemCount   int                    `json:"item_count"`
	Titles      []string               `json:"titles"`
	UploadedAt  time.Time              `json:"uploaded_at"`
}

func NewPublicUploadService(
	cfg config.UploadConfig,
	repository *repository.UploadRepository,
	receiptCodes *ReceiptCodeService,
	storageService *storage.Service,
	systemSettingService *SystemSettingService,
) *PublicUploadService {
	return &PublicUploadService{
		config:        cfg,
		repository:    repository,
		receiptCodes:  receiptCodes,
		storage:       storageService,
		systemSetting: systemSettingService,
		nowFunc:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *PublicUploadService) CreateSubmission(ctx context.Context, input PublicUploadInput) (*PublicUploadResult, error) {
	policy := s.effectivePolicy(ctx)
	normalized, err := s.normalizeInput(input, policy)
	if err != nil {
		return nil, err
	}
	if len(normalized.Files) == 0 {
		return nil, ErrInvalidUploadInput
	}
	if normalized.FolderID == "" {
		return nil, ErrUploadFolderRequired
	}

	rootFolder, err := s.repository.FindActiveFolderByID(ctx, normalized.FolderID)
	if err != nil {
		return nil, fmt.Errorf("validate upload folder: %w", err)
	}
	if rootFolder == nil || rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, ErrUploadFolderNotFound
	}
	rootFolderDisplayPath, err := s.folderDisplayPath(ctx, rootFolder)
	if err != nil {
		return nil, fmt.Errorf("resolve upload folder path: %w", err)
	}
	if err := s.resolveUploadFileNames(ctx, rootFolder, rootFolderDisplayPath, normalized.Files); err != nil {
		return nil, err
	}

	receiptCode, err := s.resolveReceiptCode(ctx, normalized.ReceiptCode)
	if err != nil {
		return nil, err
	}

	now := s.nowFunc()
	allowDirectPublish := policy.Guest.AllowDirectPublish || normalized.DirectPublish

	submissions := make([]model.Submission, 0, len(normalized.Files))
	files := make([]model.File, 0, len(normalized.Files))
	stagedPaths := make([]string, 0, len(normalized.Files))
	titles := make([]string, 0, len(normalized.Files))
	folderID := normalized.FolderID

	for _, entry := range normalized.Files {
		bufferedReader := bufio.NewReader(entry.File)
		head, inspectErr := bufferedReader.Peek(512)
		if inspectErr != nil && !errors.Is(inspectErr, io.EOF) {
			err = fmt.Errorf("inspect upload file: %w", inspectErr)
			break
		}

		detectedMIME := strings.ToLower(strings.TrimSpace(http.DetectContentType(head)))
		if !s.isAllowedMIME(detectedMIME, entry.DeclaredMIME) {
			err = ErrInvalidFileMIMEType
			break
		}

		maxFileSizeBytes := s.config.MaxFileSizeBytes
		if policy.Upload.MaxFileSizeBytes > 0 {
			maxFileSizeBytes = policy.Upload.MaxFileSizeBytes
		}
		stagedFile, saveErr := s.storage.SaveToStaging(bufferedReader, entry.Extension, maxFileSizeBytes)
		if saveErr != nil {
			switch {
			case errors.Is(saveErr, storage.ErrFileTooLarge):
				err = ErrUploadFileTooLarge
			case strings.Contains(strings.ToLower(saveErr.Error()), "empty file"):
				err = ErrUploadEmptyFile
			default:
				err = fmt.Errorf("save upload to staging: %w", saveErr)
			}
			break
		}
		stagedPaths = append(stagedPaths, stagedFile.DiskPath)

		submissionID, idErr := identity.NewID()
		if idErr != nil {
			err = fmt.Errorf("generate submission id: %w", idErr)
			break
		}
		fileID, idErr := identity.NewID()
		if idErr != nil {
			err = fmt.Errorf("generate file id: %w", idErr)
			break
		}

		submission := model.Submission{
			ID:                   submissionID,
			ReceiptCode:          receiptCode,
			TitleSnapshot:        entry.Title,
			DescriptionSnapshot:  normalized.Description,
			RelativePathSnapshot: joinSubmissionPath(rootFolderDisplayPath, entry.RelativePath),
			Status:               model.SubmissionStatusPending,
			UploaderIP:           normalized.UploaderIP,
			CreatedAt:            now,
			UpdatedAt:            now,
		}

		submissionRef := submissionID
		file := model.File{
			ID:            fileID,
			FolderID:      &folderID,
			SubmissionID:  &submissionRef,
			Title:         entry.Title,
			Description:   normalized.Description,
			OriginalName:  entry.OriginalName,
			StoredName:    stagedFile.StoredName,
			Extension:     entry.Extension,
			MimeType:      detectedMIME,
			Size:          stagedFile.Size,
			DiskPath:      stagedFile.DiskPath,
			Status:        model.ResourceStatusOffline,
			DownloadCount: 0,
			UploaderIP:    normalized.UploaderIP,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if allowDirectPublish {
			targetFolder, ensureErr := s.ensureTargetFolderForUpload(ctx, rootFolder, entry.RelativeDir, now)
			if ensureErr != nil {
				err = ensureErr
				break
			}
			finalPath, finalName, moveErr := s.storage.MoveStagedFileToFolder(stagedFile.DiskPath, *targetFolder.SourcePath, entry.OriginalName)
			if moveErr != nil {
				err = fmt.Errorf("move direct-publish upload: %w", moveErr)
				break
			}
			finalTitle := strings.TrimSuffix(finalName, filepath.Ext(finalName))
			if finalTitle == "" {
				finalTitle = finalName
			}

			file.FolderID = &targetFolder.ID
			file.Status = model.ResourceStatusActive
			file.DiskPath = finalPath
			file.StoredName = finalName
			file.OriginalName = finalName
			file.Title = finalTitle
			submission.TitleSnapshot = finalTitle
			submission.RelativePathSnapshot = joinSubmissionPath(rootFolderDisplayPath, buildRelativePath(entry.RelativeDir, finalName))
			submission.Status = model.SubmissionStatusApproved
			submission.ReviewedAt = &now
		}

		submissions = append(submissions, submission)
		files = append(files, file)
		titles = append(titles, entry.Title)
	}

	if err != nil {
		for _, path := range stagedPaths {
			if strings.TrimSpace(path) != "" {
				_ = s.storage.DeleteStagedFile(path)
			}
		}
		return nil, err
	}

	if createErr := s.repository.CreateUploadBatch(ctx, submissions, files); createErr != nil {
		for _, file := range files {
			if file.Status == model.ResourceStatusActive {
				_ = os.Remove(file.DiskPath)
				continue
			}
			_ = s.storage.DeleteStagedFile(file.DiskPath)
		}
		return nil, fmt.Errorf("persist upload submission: %w", createErr)
	}

	status := model.SubmissionStatusPending
	if allowDirectPublish {
		status = model.SubmissionStatusApproved
	}

	return &PublicUploadResult{
		ReceiptCode: receiptCode,
		Status:      status,
		ItemCount:   len(submissions),
		Titles:      titles,
		UploadedAt:  now,
	}, nil
}

type normalizedUploadInput struct {
	Description   string
	ReceiptCode   string
	FolderID      string
	UploaderIP    string
	DirectPublish bool
	Files         []normalizedUploadFile
}

type normalizedUploadFile struct {
	Title        string
	OriginalName string
	DeclaredMIME string
	RelativePath string
	RelativeDir  string
	Extension    string
	File         io.Reader
}

func (s *PublicUploadService) normalizeInput(input PublicUploadInput, policy SystemPolicy) (*normalizedUploadInput, error) {
	description := strings.TrimSpace(input.Description)
	if len([]rune(description)) > s.config.MaxDescriptionLength {
		return nil, ErrInvalidUploadInput
	}

	receiptCode, err := normalizeReceiptCode(input.ReceiptCode)
	if err != nil {
		return nil, ErrInvalidUploadInput
	}

	if len(input.Files) == 0 {
		return nil, ErrInvalidUploadInput
	}

	files := make([]normalizedUploadFile, 0, len(input.Files))
	for _, item := range input.Files {
		if isIgnoredUploadFile(item.OriginalName, item.RelativePath) {
			continue
		}

		originalName := filepath.Base(strings.TrimSpace(item.OriginalName))
		if originalName == "" || originalName == "." {
			return nil, ErrInvalidUploadInput
		}

		extension := strings.ToLower(strings.TrimSpace(filepath.Ext(originalName)))
		if !isAllowedExtension(extension, policy.Upload.AllowedExtensions) {
			return nil, ErrInvalidFileExtension
		}

		title := strings.TrimSuffix(originalName, filepath.Ext(originalName))
		if title == "" {
			title = originalName
		}

		relativePath := repository.NormalizeRelativePathForStorage(item.RelativePath)
		relativeDir := ""
		if relativePath != "" {
			relativeDir = repository.NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Dir(relativePath)))
		}

		files = append(files, normalizedUploadFile{
			Title:        title,
			OriginalName: originalName,
			DeclaredMIME: strings.ToLower(strings.TrimSpace(item.DeclaredMIME)),
			RelativePath: relativePath,
			RelativeDir:  relativeDir,
			Extension:    extension,
			File:         item.File,
		})
	}

	return &normalizedUploadInput{
		Description:   description,
		ReceiptCode:   receiptCode,
		FolderID:      strings.TrimSpace(input.FolderID),
		UploaderIP:    strings.TrimSpace(input.UploaderIP),
		DirectPublish: input.DirectPublish,
		Files:         files,
	}, nil
}

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

	reservedByDir := make(map[string]map[string]struct{})
	for _, pendingPath := range pendingPaths {
		relativeToRoot := trimRootDisplayPath(rootFolderDisplayPath, pendingPath)
		if relativeToRoot == "" {
			continue
		}
		dir := repository.NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Dir(relativeToRoot)))
		name := filepath.Base(relativeToRoot)
		if name == "" || name == "." {
			continue
		}
		reserveFileName(reservedByDir, dir, name)
	}

	for index := range files {
		entry := &files[index]
		dirKey := repository.NormalizeRelativePathForStorage(entry.RelativeDir)
		if err := s.seedReservedNamesFromDisk(reservedByDir, *rootFolder.SourcePath, dirKey); err != nil {
			return err
		}

		finalName := nextUniqueFileName(reservedByDir, dirKey, entry.OriginalName)
		entry.OriginalName = finalName
		entry.Title = strings.TrimSuffix(finalName, filepath.Ext(finalName))
		if entry.Title == "" {
			entry.Title = finalName
		}
		entry.RelativePath = buildRelativePath(dirKey, finalName)
	}

	return nil
}

func (s *PublicUploadService) seedReservedNamesFromDisk(
	reservedByDir map[string]map[string]struct{},
	rootSourcePath string,
	relativeDir string,
) error {
	if _, exists := reservedByDir[relativeDir]; exists {
		return nil
	}

	reservedByDir[relativeDir] = make(map[string]struct{})
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
		if entry.IsDir() {
			continue
		}
		reservedByDir[relativeDir][strings.ToLower(entry.Name())] = struct{}{}
	}
	return nil
}

func reserveFileName(reservedByDir map[string]map[string]struct{}, relativeDir string, fileName string) {
	if _, exists := reservedByDir[relativeDir]; !exists {
		reservedByDir[relativeDir] = make(map[string]struct{})
	}
	reservedByDir[relativeDir][strings.ToLower(fileName)] = struct{}{}
}

func nextUniqueFileName(reservedByDir map[string]map[string]struct{}, relativeDir string, originalName string) string {
	if _, exists := reservedByDir[relativeDir]; !exists {
		reservedByDir[relativeDir] = make(map[string]struct{})
	}

	originalName = filepath.Base(strings.TrimSpace(originalName))
	ext := filepath.Ext(originalName)
	base := strings.TrimSuffix(originalName, ext)
	if base == "" {
		base = originalName
	}

	for index := 0; ; index++ {
		candidate := originalName
		if index > 0 {
			candidate = base + "_" + strconv.Itoa(index) + ext
		}
		key := strings.ToLower(candidate)
		if _, exists := reservedByDir[relativeDir][key]; exists {
			continue
		}
		reservedByDir[relativeDir][key] = struct{}{}
		return candidate
	}
}

func buildRelativePath(relativeDir string, fileName string) string {
	relativeDir = repository.NormalizeRelativePathForStorage(relativeDir)
	fileName = repository.NormalizeRelativePathForStorage(fileName)
	if relativeDir == "" {
		return fileName
	}
	return relativeDir + "/" + fileName
}

func trimRootDisplayPath(rootDisplayPath string, fullPath string) string {
	rootDisplayPath = repository.NormalizeRelativePathForStorage(rootDisplayPath)
	fullPath = repository.NormalizeRelativePathForStorage(fullPath)
	if rootDisplayPath == "" {
		return fullPath
	}
	if fullPath == rootDisplayPath {
		return ""
	}
	prefix := rootDisplayPath + "/"
	if strings.HasPrefix(fullPath, prefix) {
		return strings.TrimPrefix(fullPath, prefix)
	}
	return fullPath
}

func isIgnoredUploadFile(originalName string, relativePath string) bool {
	name := strings.TrimSpace(filepath.Base(originalName))
	if name == "" && strings.TrimSpace(relativePath) != "" {
		name = strings.TrimSpace(filepath.Base(filepath.ToSlash(relativePath)))
	}
	return strings.EqualFold(name, ".DS_Store")
}

func (s *PublicUploadService) ensureTargetFolderForUpload(ctx context.Context, rootFolder *model.Folder, relativeDir string, now time.Time) (*model.Folder, error) {
	if relativeDir == "" {
		return rootFolder, nil
	}
	if rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, ErrUploadFolderNotFound
	}
	if err := s.storage.EnsureManagedDirectory(filepath.Join(*rootFolder.SourcePath, filepath.FromSlash(relativeDir))); err != nil {
		return nil, fmt.Errorf("ensure upload target directory: %w", err)
	}

	var targetFolder *model.Folder
	err := s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		leaf, ensureErr := repository.EnsureActiveFolderPathTx(tx, rootFolder, relativeDir, now)
		if ensureErr != nil {
			return ensureErr
		}
		targetFolder = leaf
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ensure upload folder path: %w", err)
	}
	return targetFolder, nil
}

func (s *PublicUploadService) folderDisplayPath(ctx context.Context, folder *model.Folder) (string, error) {
	if folder == nil {
		return "", nil
	}

	segments := []string{strings.TrimSpace(folder.Name)}
	parentID := folder.ParentID

	for parentID != nil && strings.TrimSpace(*parentID) != "" {
		parent, err := s.repository.FindActiveFolderByID(ctx, *parentID)
		if err != nil {
			return "", fmt.Errorf("find ancestor folder: %w", err)
		}
		if parent == nil {
			break
		}
		segments = append(segments, strings.TrimSpace(parent.Name))
		parentID = parent.ParentID
	}

	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}

	filtered := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = repository.NormalizeRelativePathForStorage(segment)
		if segment == "" {
			continue
		}
		filtered = append(filtered, segment)
	}

	return strings.Join(filtered, "/"), nil
}

func joinSubmissionPath(basePath, relativePath string) string {
	basePath = repository.NormalizeRelativePathForStorage(basePath)
	relativePath = repository.NormalizeRelativePathForStorage(relativePath)

	switch {
	case basePath == "":
		return relativePath
	case relativePath == "":
		return basePath
	default:
		return basePath + "/" + relativePath
	}
}

func (s *PublicUploadService) effectivePolicy(ctx context.Context) SystemPolicy {
	if s.systemSetting == nil {
		return defaultSystemPolicy(s.config)
	}

	policy, err := s.systemSetting.GetPolicy(ctx)
	if err != nil || policy == nil {
		return defaultSystemPolicy(s.config)
	}

	return *policy
}

func (s *PublicUploadService) resolveReceiptCode(ctx context.Context, receiptCode string) (string, error) {
	return s.receiptCodes.ResolveForSession(ctx, receiptCode)
}

// retryCreateWithGeneratedReceipt is kept for backward compatibility but
// receipt code conflicts should no longer occur since the unique constraint
// was replaced with a regular index.

func isAllowedExtension(extension string, allowedExtensions []string) bool {
	if len(allowedExtensions) == 0 {
		return true
	}
	for _, allowed := range allowedExtensions {
		if strings.EqualFold(extension, strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func (s *PublicUploadService) isAllowedMIME(detectedMIME, declaredMIME string) bool {
	if len(s.config.AllowedMIMETypes) == 0 {
		return true
	}
	for _, allowed := range s.config.AllowedMIMETypes {
		if detectedMIME == allowed || declaredMIME == allowed {
			return true
		}
	}
	return false
}
