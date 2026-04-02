package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

var (
	ErrInvalidPublicFileQuery = errors.New("invalid public file query")
	ErrFolderNotFound         = errors.New("folder not found")
)

const (
	defaultPublicFilePage     = 1
	defaultPublicFilePageSize = 20
	maxPublicFilePageSize     = 100
)

type PublicCatalogService struct {
	repository *repository.PublicCatalogRepository
}

type PublicFolderFileListInput struct {
	FolderID string
	Page     int
	PageSize int
	Sort     string
}

type PublicFolderFileListResult struct {
	Items    []PublicFileItem `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int64            `json:"total"`
}

type PublicFileFeedResult struct {
	Items []PublicFileItem `json:"items"`
}

type PublicFileItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Extension     string    `json:"extension"`
	UploadedAt    time.Time `json:"uploaded_at"`
	DownloadCount int64     `json:"download_count"`
	Size          int64     `json:"size"`
}

type PublicFolderItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	UpdatedAt     time.Time `json:"updated_at"`
	FileCount     int64     `json:"file_count"`
	DownloadCount int64     `json:"download_count"`
	TotalSize     int64     `json:"total_size"`
}

type PublicFolderBreadcrumbItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PublicFolderDetail struct {
	ID            string                       `json:"id"`
	Name          string                       `json:"name"`
	Description   string                       `json:"description"`
	ParentID      *string                      `json:"parent_id"`
	Breadcrumbs   []PublicFolderBreadcrumbItem `json:"breadcrumbs"`
	FileCount     int64                        `json:"file_count"`
	DownloadCount int64                        `json:"download_count"`
	TotalSize     int64                        `json:"total_size"`
	UpdatedAt     time.Time                    `json:"updated_at"`
}

func NewPublicCatalogService(repository *repository.PublicCatalogRepository) *PublicCatalogService {
	return &PublicCatalogService{repository: repository}
}

func (s *PublicCatalogService) ListPublicFolderFiles(ctx context.Context, input PublicFolderFileListInput) (*PublicFolderFileListResult, error) {
	normalized, err := normalizePublicFolderFileListInput(input)
	if err != nil {
		return nil, err
	}

	exists, err := s.repository.FolderExists(ctx, normalized.FolderID)
	if err != nil {
		return nil, fmt.Errorf("validate folder: %w", err)
	}
	if !exists {
		return nil, ErrFolderNotFound
	}

	files, total, err := s.repository.ListPublicFolderFiles(ctx, repository.PublicFolderFileListQuery{
		FolderID: normalized.FolderID,
		Offset:   (normalized.Page - 1) * normalized.PageSize,
		Limit:    normalized.PageSize,
		OrderBy:  normalized.OrderBy,
	})
	if err != nil {
		return nil, fmt.Errorf("list public folder files: %w", err)
	}

	return &PublicFolderFileListResult{
		Items:    mapPublicFileItems(files),
		Page:     normalized.Page,
		PageSize: normalized.PageSize,
		Total:    total,
	}, nil
}

func (s *PublicCatalogService) ListHotFiles(ctx context.Context, limit int) (*PublicFileFeedResult, error) {
	return s.listManagedFileFeed(ctx, limit, []string{"download_count DESC", "created_at DESC", "id DESC"})
}

func (s *PublicCatalogService) ListLatestFiles(ctx context.Context, limit int) (*PublicFileFeedResult, error) {
	return s.listManagedFileFeed(ctx, limit, []string{"created_at DESC", "id DESC"})
}

func (s *PublicCatalogService) ListPublicFolders(ctx context.Context, parentID string) ([]PublicFolderItem, error) {
	var parentPtr *string
	if trimmed := strings.TrimSpace(parentID); trimmed != "" {
		exists, err := s.repository.FolderExists(ctx, trimmed)
		if err != nil {
			return nil, fmt.Errorf("validate parent folder: %w", err)
		}
		if !exists {
			return nil, ErrFolderNotFound
		}
		parentPtr = &trimmed
	}

	rows, err := s.repository.ListPublicFolders(ctx, parentPtr)
	if err != nil {
		return nil, fmt.Errorf("list public folders: %w", err)
	}

	items := make([]PublicFolderItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PublicFolderItem{
			ID:            row.ID,
			Name:          row.Name,
			Description:   row.Description,
			UpdatedAt:     row.UpdatedAt,
			FileCount:     row.FileCount,
			DownloadCount: row.DownloadCount,
			TotalSize:     row.TotalSize,
		})
	}

	return items, nil
}

func (s *PublicCatalogService) GetPublicFolderDetail(ctx context.Context, folderID string) (*PublicFolderDetail, error) {
	trimmed := strings.TrimSpace(folderID)
	if trimmed == "" {
		return nil, ErrFolderNotFound
	}

	current, err := s.repository.FindPublicFolderByID(ctx, trimmed)
	if err != nil {
		return nil, fmt.Errorf("find public folder: %w", err)
	}
	if current == nil {
		return nil, ErrFolderNotFound
	}

	breadcrumbs := []PublicFolderBreadcrumbItem{{ID: current.ID, Name: current.Name}}
	parentID := current.ParentID
	for parentID != nil {
		parent, err := s.repository.FindPublicFolderByID(ctx, *parentID)
		if err != nil {
			return nil, fmt.Errorf("find public folder ancestor: %w", err)
		}
		if parent == nil {
			return nil, ErrFolderNotFound
		}
		breadcrumbs = append(breadcrumbs, PublicFolderBreadcrumbItem{
			ID:   parent.ID,
			Name: parent.Name,
		})
		parentID = parent.ParentID
	}

	for i, j := 0, len(breadcrumbs)-1; i < j; i, j = i+1, j-1 {
		breadcrumbs[i], breadcrumbs[j] = breadcrumbs[j], breadcrumbs[i]
	}

	return &PublicFolderDetail{
		ID:            current.ID,
		Name:          current.Name,
		Description:   current.Description,
		ParentID:      current.ParentID,
		Breadcrumbs:   breadcrumbs,
		FileCount:     current.FileCount,
		DownloadCount: current.DownloadCount,
		TotalSize:     current.TotalSize,
		UpdatedAt:     current.UpdatedAt,
	}, nil
}

type normalizedPublicFolderFileListInput struct {
	FolderID string
	Page     int
	PageSize int
	OrderBy  []string
}

func normalizePublicFolderFileListInput(input PublicFolderFileListInput) (*normalizedPublicFolderFileListInput, error) {
	folderID := strings.TrimSpace(input.FolderID)
	if folderID == "" {
		return nil, ErrInvalidPublicFileQuery
	}

	page := input.Page
	if page == 0 {
		page = defaultPublicFilePage
	}
	if page < 1 {
		return nil, ErrInvalidPublicFileQuery
	}

	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = defaultPublicFilePageSize
	}
	if pageSize < 1 || pageSize > maxPublicFilePageSize {
		return nil, ErrInvalidPublicFileQuery
	}

	orderBy, err := resolvePublicFileSort(input.Sort)
	if err != nil {
		return nil, err
	}

	return &normalizedPublicFolderFileListInput{
		FolderID: folderID,
		Page:     page,
		PageSize: pageSize,
		OrderBy:  orderBy,
	}, nil
}

func resolvePublicFileSort(sort string) ([]string, error) {
	switch strings.TrimSpace(sort) {
	case "", "created_at_desc":
		return []string{"created_at DESC", "id DESC"}, nil
	case "download_count_desc":
		return []string{"download_count DESC", "created_at DESC", "id DESC"}, nil
	case "name_asc":
		return []string{"name ASC", "created_at DESC", "id DESC"}, nil
	default:
		return nil, ErrInvalidPublicFileQuery
	}
}

func (s *PublicCatalogService) listManagedFileFeed(ctx context.Context, limit int, orderBy []string) (*PublicFileFeedResult, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 20
	}
	if normalizedLimit > maxPublicFilePageSize {
		normalizedLimit = maxPublicFilePageSize
	}

	files, err := s.repository.ListManagedFileFeed(ctx, repository.PublicFileFeedQuery{
		Limit:   normalizedLimit,
		OrderBy: orderBy,
	})
	if err != nil {
		return nil, fmt.Errorf("list managed file feed: %w", err)
	}

	return &PublicFileFeedResult{
		Items: mapPublicFileItems(files),
	}, nil
}

func mapPublicFileItems(files []model.File) []PublicFileItem {
	items := make([]PublicFileItem, 0, len(files))
	for _, file := range files {
		items = append(items, PublicFileItem{
			ID:            file.ID,
			Name:          file.Name,
			Description:   file.Description,
			Extension:     file.Extension,
			UploadedAt:    file.CreatedAt,
			DownloadCount: file.DownloadCount,
			Size:          file.Size,
		})
	}
	return items
}
