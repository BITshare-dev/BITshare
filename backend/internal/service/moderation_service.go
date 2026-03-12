package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
)

var (
	ErrSubmissionNotPending = errors.New("submission is not pending")
	ErrSubmissionMissing    = errors.New("submission not found")
	ErrStagedFileMissing    = errors.New("staged file not found")
	ErrRejectReasonRequired = errors.New("reject reason is required")
	ErrApproveNoFolder      = errors.New("cannot approve: file has no target folder")
	ErrApproveFolderMissing = errors.New("cannot approve: target folder not found or has no source path")
)

type ModerationService struct {
	repository *repository.ModerationRepository
	storage    *storage.Service
	search     *SearchService
	nowFunc    func() time.Time
}

type PendingSubmissionItem struct {
	SubmissionID  string                 `json:"submission_id"`
	ReceiptCode   string                 `json:"receipt_code"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description"`
	Status        model.SubmissionStatus `json:"status"`
	UploadedAt    time.Time              `json:"uploaded_at"`
	FileID        string                 `json:"file_id"`
	FileName      string                 `json:"file_name"`
	FileSize      int64                  `json:"file_size"`
	FileMimeType  string                 `json:"file_mime_type"`
	DownloadCount int64                  `json:"download_count"`
}

type ReviewResult struct {
	SubmissionID string                 `json:"submission_id"`
	Status       model.SubmissionStatus `json:"status"`
	ReviewedAt   time.Time              `json:"reviewed_at"`
	RejectReason string                 `json:"reject_reason,omitempty"`
}

func NewModerationService(repository *repository.ModerationRepository, storageService *storage.Service, searchService *SearchService) *ModerationService {
	return &ModerationService{
		repository: repository,
		storage:    storageService,
		search:     searchService,
		nowFunc:    func() time.Time { return time.Now().UTC() },
	}
}

func (s *ModerationService) ListPendingSubmissions(ctx context.Context) ([]PendingSubmissionItem, error) {
	rows, err := s.repository.ListPendingSubmissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending submissions: %w", err)
	}

	items := make([]PendingSubmissionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PendingSubmissionItem{
			SubmissionID:  row.SubmissionID,
			ReceiptCode:   row.ReceiptCode,
			Title:         row.Title,
			Description:   row.Description,
			Status:        row.Status,
			UploadedAt:    row.CreatedAt,
			FileID:        row.FileID,
			FileName:      row.FileName,
			FileSize:      row.FileSize,
			FileMimeType:  row.FileMimeType,
			DownloadCount: row.DownloadCount,
		})
	}

	return items, nil
}

func (s *ModerationService) ApproveSubmission(ctx context.Context, submissionID string, adminID string, operatorIP string) (*ReviewResult, error) {
	record, err := s.repository.FindPendingSubmission(ctx, strings.TrimSpace(submissionID))
	if err != nil {
		return nil, fmt.Errorf("find submission for approval: %w", err)
	}
	if record == nil {
		return nil, ErrSubmissionMissing
	}
	if record.Submission.Status != model.SubmissionStatusPending {
		return nil, ErrSubmissionNotPending
	}

	exists, err := s.storage.StagedFileExists(record.File.DiskPath)
	if err != nil {
		return nil, fmt.Errorf("validate staged file: %w", err)
	}
	if !exists {
		return nil, ErrStagedFileMissing
	}

	// Resolve the target folder's disk directory.
	if record.File.FolderID == nil {
		return nil, ErrApproveNoFolder
	}
	folder, err := s.repository.FindFolderByID(ctx, *record.File.FolderID)
	if err != nil {
		return nil, fmt.Errorf("find target folder: %w", err)
	}
	if folder == nil || folder.SourcePath == nil {
		return nil, ErrApproveFolderMissing
	}

	// Move staged file into the folder's physical directory.
	finalPath, finalName, err := s.storage.MoveStagedFileToFolder(record.File.DiskPath, *folder.SourcePath, record.File.OriginalName)
	if err != nil {
		return nil, fmt.Errorf("move staged file to folder: %w", err)
	}

	reviewedAt := s.nowFunc()
	if err := s.repository.ApproveSubmission(ctx, record.Submission.ID, adminID, operatorIP, reviewedAt, finalPath, finalName); err != nil {
		// Rollback: move the file back to staging.
		if _, rollbackErr := s.storage.MoveFileBackToStaging(finalPath, record.File.StoredName); rollbackErr != nil {
			return nil, fmt.Errorf("approve submission failed (%v); rollback failed: %w", err, rollbackErr)
		}
		return nil, fmt.Errorf("approve submission: %w", err)
	}

	// Update FTS5 search index for the newly approved file.
	if s.search != nil {
		_ = s.search.IndexFile(ctx, record.File.ID, record.File.Title)
	}

	return &ReviewResult{
		SubmissionID: record.Submission.ID,
		Status:       model.SubmissionStatusApproved,
		ReviewedAt:   reviewedAt,
	}, nil
}

func (s *ModerationService) RejectSubmission(ctx context.Context, submissionID string, adminID string, operatorIP string, rejectReason string) (*ReviewResult, error) {
	rejectReason = strings.TrimSpace(rejectReason)
	if rejectReason == "" {
		return nil, ErrRejectReasonRequired
	}

	record, err := s.repository.FindPendingSubmission(ctx, strings.TrimSpace(submissionID))
	if err != nil {
		return nil, fmt.Errorf("find submission for rejection: %w", err)
	}
	if record == nil {
		return nil, ErrSubmissionMissing
	}
	if record.Submission.Status != model.SubmissionStatusPending {
		return nil, ErrSubmissionNotPending
	}

	exists, err := s.storage.StagedFileExists(record.File.DiskPath)
	if err != nil {
		return nil, fmt.Errorf("validate staged file: %w", err)
	}
	if exists {
		if err := s.storage.DeleteStagedFile(record.File.DiskPath); err != nil {
			return nil, fmt.Errorf("delete staged file: %w", err)
		}
	}

	reviewedAt := s.nowFunc()
	if err := s.repository.RejectSubmission(ctx, record.Submission.ID, adminID, operatorIP, reviewedAt, rejectReason); err != nil {
		return nil, fmt.Errorf("reject submission: %w", err)
	}

	return &ReviewResult{
		SubmissionID: record.Submission.ID,
		Status:       model.SubmissionStatusRejected,
		ReviewedAt:   reviewedAt,
		RejectReason: rejectReason,
	}, nil
}
