package service

import (
	"context"
	"errors"
	"time"

	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
)

var (
	ErrManagedFileNotFound   = errors.New("managed file not found")
	ErrManagedFileConflict   = errors.New("managed file conflict")
	ErrManagedFolderNotFound = errors.New("managed folder not found")
	ErrManagedFolderConflict = errors.New("managed folder conflict")
	ErrInvalidResourceEdit   = errors.New("invalid resource edit")
)

type ResourceManagementService struct {
	repo    *repository.ResourceManagementRepository
	storage *storage.Service
	nowFunc func() time.Time
}

type ManagedFileItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Extension     string    `json:"extension"`
	Size          int64     `json:"size"`
	DownloadCount int64     `json:"download_count"`
	FolderName    string    `json:"folder_name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ListManagedFilesInput struct {
	Query string
}

type UpdateManagedFileInput struct {
	Name        string
	Description string
	OperatorID  string
	OperatorIP  string
}

type UpdateManagedFolderDescriptionInput struct {
	Name        string
	Description string
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

func (s *ResourceManagementService) ListFiles(ctx context.Context, input ListManagedFilesInput) ([]ManagedFileItem, error) {
	rows, err := s.repo.ListFiles(ctx, input.Query)
	if err != nil {
		return nil, err
	}
	items := make([]ManagedFileItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ManagedFileItem{
			ID:            row.ID,
			Name:          row.Name,
			Description:   row.Description,
			Extension:     row.Extension,
			Size:          row.Size,
			DownloadCount: row.DownloadCount,
			FolderName:    row.FolderName,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		})
	}
	return items, nil
}
