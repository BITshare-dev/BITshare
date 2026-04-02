package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

var (
	ErrInvalidUploadInput   = errors.New("invalid upload input")
	ErrUploadReceiptExists  = errors.New("receipt code already exists")
	ErrUploadTooLarge       = errors.New("upload too large")
	ErrUploadEmptyFile      = errors.New("upload file is empty")
	ErrUploadNameConflict   = errors.New("upload name conflict")
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
	Description string
	ReceiptCode string
	FolderID    string
	UploaderIP  string
	Files       []PublicUploadFileInput
}

type PublicUploadFileInput struct {
	Name         string
	RelativePath string
	File         io.Reader
}

type PublicUploadResult struct {
	ReceiptCode string                 `json:"receipt_code"`
	Status      model.SubmissionStatus `json:"status"`
	ItemCount   int                    `json:"item_count"`
	Names       []string               `json:"names"`
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
	actor, canDirectPublish := publicUploadActorFromContext(ctx)
	normalized, err := s.normalizeInput(input)
	if err != nil {
		return nil, err
	}
	if len(normalized.Files) == 0 {
		return nil, ErrInvalidUploadInput
	}
	if normalized.FolderID == "" {
		return nil, ErrUploadFolderRequired
	}

	rootFolder, err := s.repository.FindManagedFolderByID(ctx, normalized.FolderID)
	if err != nil {
		return nil, fmt.Errorf("validate upload folder: %w", err)
	}
	if rootFolder == nil || rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, ErrUploadFolderNotFound
	}

	rootFolderDisplayPath, err := s.repository.BuildFolderDisplayPath(ctx, &rootFolder.ID)
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

	submissions := make([]model.Submission, 0, len(normalized.Files))
	stagedPaths := make([]string, 0, len(normalized.Files))
	names := make([]string, 0, len(normalized.Files))
	folderID := normalized.FolderID

	for _, entry := range normalized.Files {
		bufferedReader := bufio.NewReader(entry.File)
		head, inspectErr := bufferedReader.Peek(512)
		if inspectErr != nil && !errors.Is(inspectErr, io.EOF) {
			err = fmt.Errorf("inspect upload file: %w", inspectErr)
			break
		}

		detectedMIME := strings.ToLower(strings.TrimSpace(http.DetectContentType(head)))

		maxUploadTotalBytes := s.config.MaxUploadTotalBytes
		if policy.Upload.MaxUploadTotalBytes > 0 {
			maxUploadTotalBytes = policy.Upload.MaxUploadTotalBytes
		}
		stagedFile, saveErr := s.storage.SaveToStaging(bufferedReader, entry.Extension, maxUploadTotalBytes)
		if saveErr != nil {
			switch {
			case errors.Is(saveErr, storage.ErrFileTooLarge):
				err = ErrUploadTooLarge
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

		folderRef := folderID
		submission := model.Submission{
			ID:           submissionID,
			ReceiptCode:  receiptCode,
			FolderID:     &folderRef,
			Name:         entry.Name,
			Description:  normalized.Description,
			RelativePath: entry.RelativePath,
			Extension:    entry.Extension,
			MimeType:     detectedMIME,
			Size:         stagedFile.Size,
			StagingPath:  stagedFile.DiskPath,
			Status:       model.SubmissionStatusPending,
			UploaderIP:   normalized.UploaderIP,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		submissions = append(submissions, submission)
		names = append(names, submission.Name)
	}

	if err != nil {
		s.cleanupStagedPaths(stagedPaths)
		return nil, err
	}

	if canDirectPublish {
		result, publishErr := s.publishDirectUploadBatch(ctx, rootFolder, submissions, actor.AdminID, normalized.UploaderIP, now)
		if publishErr != nil {
			s.cleanupStagedSubmissions(submissions)
			return nil, publishErr
		}
		return result, nil
	}

	if createErr := s.repository.CreateUploadBatch(ctx, submissions); createErr != nil {
		s.cleanupStagedSubmissions(submissions)
		return nil, fmt.Errorf("persist upload submission: %w", createErr)
	}

	return &PublicUploadResult{
		ReceiptCode: receiptCode,
		Status:      model.SubmissionStatusPending,
		ItemCount:   len(submissions),
		Names:       names,
		UploadedAt:  now,
	}, nil
}

type directPublishedUpload struct {
	submission        model.Submission
	finalPath         string
	finalName         string
	finalRelativePath string
}

func (s *PublicUploadService) publishDirectUploadBatch(
	ctx context.Context,
	rootFolder *model.Folder,
	submissions []model.Submission,
	adminID string,
	operatorIP string,
	reviewedAt time.Time,
) (*PublicUploadResult, error) {
	if rootFolder == nil || rootFolder.SourcePath == nil || strings.TrimSpace(*rootFolder.SourcePath) == "" {
		return nil, ErrUploadFolderNotFound
	}

	published := make([]directPublishedUpload, 0, len(submissions))
	items := make([]repository.ApprovedUploadBatchItem, 0, len(submissions))
	names := make([]string, 0, len(submissions))
	rootSourcePath := strings.TrimSpace(*rootFolder.SourcePath)

	for _, submission := range submissions {
		relativeDir := repository.NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Dir(submission.RelativePath)))
		targetDir := rootSourcePath
		if relativeDir != "" {
			targetDir = filepath.Join(rootSourcePath, filepath.FromSlash(relativeDir))
		}
		if err := s.storage.EnsureManagedDirectory(targetDir); err != nil {
			rollbackErr := s.rollbackDirectPublishedUploads(published)
			if rollbackErr != nil {
				return nil, fmt.Errorf("ensure direct upload target directory failed (%v); rollback failed: %w", err, rollbackErr)
			}
			return nil, fmt.Errorf("ensure direct upload target directory: %w", err)
		}

		finalPath, finalName, err := s.storage.MoveStagedFileToFolder(submission.StagingPath, targetDir, submission.Name)
		if err != nil {
			rollbackErr := s.rollbackDirectPublishedUploads(published)
			if rollbackErr != nil {
				return nil, fmt.Errorf("direct upload move failed (%v); rollback failed: %w", err, rollbackErr)
			}
			return nil, fmt.Errorf("move direct upload into managed folder: %w", err)
		}

		finalRelativePath := replaceRelativePathBase(submission.RelativePath, finalName)
		published = append(published, directPublishedUpload{
			submission:        submission,
			finalPath:         finalPath,
			finalName:         finalName,
			finalRelativePath: finalRelativePath,
		})
		items = append(items, repository.ApprovedUploadBatchItem{
			SubmissionID:      submission.ID,
			FinalName:         finalName,
			FinalRelativePath: finalRelativePath,
		})
		names = append(names, finalName)
	}

	if err := s.repository.CreateApprovedUploadBatch(ctx, rootFolder, submissions, items, adminID, operatorIP, reviewedAt); err != nil {
		rollbackErr := s.rollbackDirectPublishedUploads(published)
		if rollbackErr != nil {
			return nil, fmt.Errorf("persist direct upload failed (%v); rollback failed: %w", err, rollbackErr)
		}
		return nil, fmt.Errorf("persist direct upload: %w", err)
	}

	return &PublicUploadResult{
		ReceiptCode: submissions[0].ReceiptCode,
		Status:      model.SubmissionStatusApproved,
		ItemCount:   len(submissions),
		Names:       names,
		UploadedAt:  reviewedAt,
	}, nil
}

func (s *PublicUploadService) rollbackDirectPublishedUploads(published []directPublishedUpload) error {
	for i := len(published) - 1; i >= 0; i-- {
		entry := published[i]
		if _, err := s.storage.MoveFileBackToStaging(entry.finalPath, entry.submission.StagingPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *PublicUploadService) cleanupStagedSubmissions(submissions []model.Submission) {
	for _, submission := range submissions {
		if strings.TrimSpace(submission.StagingPath) != "" {
			_ = s.storage.DeleteStagedFile(submission.StagingPath)
		}
	}
}

func (s *PublicUploadService) cleanupStagedPaths(paths []string) {
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			_ = s.storage.DeleteStagedFile(path)
		}
	}
}

func (s *PublicUploadService) MaxUploadTotalBytes(ctx context.Context) int64 {
	policy := s.effectivePolicy(ctx)
	if policy.Upload.MaxUploadTotalBytes > 0 {
		return policy.Upload.MaxUploadTotalBytes
	}
	return s.config.MaxUploadTotalBytes
}
