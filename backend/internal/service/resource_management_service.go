package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/identity"
)

var (
	ErrManagedFileNotFound = errors.New("managed file not found")
	ErrInvalidResourceEdit = errors.New("invalid resource edit")
)

type ResourceManagementService struct {
	repo     *repository.ResourceManagementRepository
	storage  *storage.Service
	settings *SystemSettingService
	nowFunc  func() time.Time
}

type ManagedFileItem struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	Description   string               `json:"description"`
	OriginalName  string               `json:"original_name"`
	Status        model.ResourceStatus `json:"status"`
	Size          int64                `json:"size"`
	DownloadCount int64                `json:"download_count"`
	FolderName    string               `json:"folder_name"`
	Tags          []string             `json:"tags"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

type ListManagedFilesInput struct {
	Query  string
	Status string
}

type UpdateManagedFileInput struct {
	Title       string
	Description string
	Tags        []string
	OperatorID  string
	OperatorIP  string
}

func NewResourceManagementService(repo *repository.ResourceManagementRepository, storageService *storage.Service) *ResourceManagementService {
	return &ResourceManagementService{
		repo:    repo,
		storage: storageService,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
}

func NewResourceManagementServiceWithSettings(repo *repository.ResourceManagementRepository, storageService *storage.Service, settings *SystemSettingService) *ResourceManagementService {
	service := NewResourceManagementService(repo, storageService)
	service.settings = settings
	return service
}

func (s *ResourceManagementService) ListFiles(ctx context.Context, input ListManagedFilesInput) ([]ManagedFileItem, error) {
	rows, err := s.repo.ListFiles(ctx, input.Query, input.Status)
	if err != nil {
		return nil, err
	}
	fileIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		fileIDs = append(fileIDs, row.ID)
	}
	tagRows, err := s.repo.ListFileTags(ctx, fileIDs)
	if err != nil {
		return nil, err
	}
	tagsByFile := make(map[string][]string, len(tagRows))
	for _, row := range tagRows {
		tagsByFile[row.FileID] = append(tagsByFile[row.FileID], row.TagName)
	}

	items := make([]ManagedFileItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ManagedFileItem{
			ID:            row.ID,
			Title:         row.Title,
			Description:   row.Description,
			OriginalName:  row.OriginalName,
			Status:        row.Status,
			Size:          row.Size,
			DownloadCount: row.DownloadCount,
			FolderName:    row.FolderName,
			Tags:          tagsByFile[row.ID],
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		})
	}
	return items, nil
}

func (s *ResourceManagementService) UpdateFile(ctx context.Context, fileID string, input UpdateManagedFileInput) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return ErrManagedFileNotFound
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return ErrInvalidResourceEdit
	}
	description := strings.TrimSpace(input.Description)
	tags, err := normalizeTags(input.Tags, 20, 32)
	if err != nil {
		return ErrInvalidResourceEdit
	}
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate resource update log id: %w", err)
	}
	if err := s.repo.UpdateFileMetadataAndTags(ctx, fileID, title, description, tags, input.OperatorID, input.OperatorIP, logID, s.nowFunc()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFileNotFound
		}
		return fmt.Errorf("update managed file: %w", err)
	}
	return nil
}

func (s *ResourceManagementService) OfflineFile(ctx context.Context, fileID string, operatorID string, operatorIP string) error {
	current, err := s.repo.FindFileByID(ctx, strings.TrimSpace(fileID))
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate resource offline log id: %w", err)
	}
	if err := s.repo.UpdateFileStatusWithLog(ctx, current.ID, model.ResourceStatusOffline, nil, current.DiskPath, operatorID, operatorIP, "resource_offlined", current.Title, logID, s.nowFunc()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFileNotFound
		}
		return fmt.Errorf("offline managed file: %w", err)
	}
	return nil
}

func (s *ResourceManagementService) DeleteFile(ctx context.Context, fileID string, operatorID string, operatorIP string) error {
	current, err := s.repo.FindFileByID(ctx, strings.TrimSpace(fileID))
	if err != nil {
		return err
	}
	if current == nil {
		return ErrManagedFileNotFound
	}

	newPath, err := s.storage.MoveManagedFileToTrash(current.DiskPath)
	if err != nil {
		return fmt.Errorf("move managed file to trash: %w", err)
	}

	now := s.nowFunc()
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate resource delete log id: %w", err)
	}
	if err := s.repo.UpdateFileStatusWithLog(ctx, current.ID, model.ResourceStatusDeleted, &now, newPath, operatorID, operatorIP, "resource_deleted", current.Title, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedFileNotFound
		}
		return fmt.Errorf("delete managed file: %w", err)
	}
	return nil
}

func (s *ResourceManagementService) PublicUpdateFile(ctx context.Context, fileID string, input UpdateManagedFileInput) error {
	policy, err := s.guestPolicy(ctx)
	if err != nil {
		return err
	}
	if !policy.AllowGuestEditTitle && !policy.AllowGuestEditTags && !policy.AllowGuestEditDescription {
		return ErrInvalidResourceEdit
	}

	current, err := s.repo.FindFileByID(ctx, strings.TrimSpace(fileID))
	if err != nil {
		return err
	}
	if current == nil || current.Status != model.ResourceStatusActive {
		return ErrManagedFileNotFound
	}

	merged := input
	if !policy.AllowGuestEditTitle {
		merged.Title = current.Title
	}
	if !policy.AllowGuestEditDescription {
		merged.Description = current.Description
	}
	if !policy.AllowGuestEditTags {
		tagRows, err := s.repo.ListFileTags(ctx, []string{current.ID})
		if err != nil {
			return fmt.Errorf("list current file tags: %w", err)
		}
		merged.Tags = make([]string, 0, len(tagRows))
		for _, row := range tagRows {
			merged.Tags = append(merged.Tags, row.TagName)
		}
	}

	merged.OperatorID = ""
	return s.UpdateFile(ctx, fileID, merged)
}

func (s *ResourceManagementService) PublicDeleteFile(ctx context.Context, fileID string, operatorIP string) error {
	policy, err := s.guestPolicy(ctx)
	if err != nil {
		return err
	}
	if !policy.AllowGuestResourceDelete {
		return ErrInvalidResourceEdit
	}

	current, err := s.repo.FindFileByID(ctx, strings.TrimSpace(fileID))
	if err != nil {
		return err
	}
	if current == nil || current.Status != model.ResourceStatusActive {
		return ErrManagedFileNotFound
	}

	return s.DeleteFile(ctx, fileID, "", operatorIP)
}

func (s *ResourceManagementService) guestPolicy(ctx context.Context) (GuestPolicy, error) {
	if s.settings == nil {
		return GuestPolicy{}, nil
	}

	policy, err := s.settings.GetPolicy(ctx)
	if err != nil {
		return GuestPolicy{}, fmt.Errorf("load system policy: %w", err)
	}
	if policy == nil {
		return GuestPolicy{}, nil
	}
	return policy.Guest, nil
}
