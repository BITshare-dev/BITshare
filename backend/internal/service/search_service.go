package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/search"
)

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	ErrSearchQueryEmpty   = errors.New("search query is empty")
	ErrSearchQueryTooLong = errors.New("search query exceeds maximum length")
	ErrSearchInvalidInput = errors.New("invalid search parameters")
)

const (
	defaultSearchPage     = 1
	defaultSearchPageSize = 20
	maxSearchPageSize     = 100
	maxSearchQueryLength  = 200
)

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// SearchService implements the public search use-case:
//   - keyword search (FTS5 with LIKE fallback)
//   - tag filtering (AND semantics)
//   - tag inheritance (files inherit ancestor-folder tags)
//   - folder-scoped search
//   - relevance + download_count ordering
type SearchService struct {
	searchRepo *repository.SearchRepository
	tagRepo    *repository.TagRepository
}

func NewSearchService(searchRepo *repository.SearchRepository, tagRepo *repository.TagRepository) *SearchService {
	return &SearchService{
		searchRepo: searchRepo,
		tagRepo:    tagRepo,
	}
}

// ---------------------------------------------------------------------------
// Input / Output
// ---------------------------------------------------------------------------

// SearchInput is the external request from the handler layer.
type SearchInput struct {
	Keyword  string   // raw user input
	Tags     []string // raw tag names to filter by
	FolderID string   // optional folder scope
	Page     int
	PageSize int
}

// SearchResult is the response delivered to the handler layer.
type SearchResult struct {
	Items    []SearchResultItem `json:"items"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int64              `json:"total"`
}

// SearchResultItem represents a single file or folder in the search results.
type SearchResultItem struct {
	EntityType    string   `json:"entity_type"` // "file" | "folder"
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Tags          []string `json:"tags"`
	Size          int64    `json:"size,omitempty"`
	DownloadCount int64    `json:"download_count,omitempty"`
	UploadedAt    *time.Time `json:"uploaded_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Core search
// ---------------------------------------------------------------------------

func (s *SearchService) Search(ctx context.Context, input SearchInput) (*SearchResult, error) {
	// --- 1. Validate & normalise -----------------------------------------
	page, pageSize, err := normalizeSearchPagination(input.Page, input.PageSize)
	if err != nil {
		return nil, err
	}

	keyword := strings.TrimSpace(input.Keyword)
	if len([]rune(keyword)) > maxSearchQueryLength {
		return nil, ErrSearchQueryTooLong
	}

	// Sanitize keyword for FTS5
	fts5Query, hasFTS := search.SanitizeQuery(keyword)

	// Normalize tag filters
	var tagFilters []string
	for _, raw := range input.Tags {
		normalized, ok := search.SanitizeTagName(raw)
		if ok {
			tagFilters = append(tagFilters, normalized)
		}
	}

	if !hasFTS && len(tagFilters) == 0 {
		return nil, ErrSearchQueryEmpty
	}

	// --- 2. Resolve folder scope -----------------------------------------
	var scopeFolderIDs []string
	if trimmed := strings.TrimSpace(input.FolderID); trimmed != "" {
		ids, err := s.searchRepo.GetDescendantFolderIDs(ctx, trimmed)
		if err != nil {
			return nil, fmt.Errorf("resolve folder scope: %w", err)
		}
		scopeFolderIDs = ids
	}

	// --- 3. Expand tag inheritance into FTS5 index -----------------------
	// Tag inheritance: when user searches by tag, files inherit their
	// parent-folder tags. Since tags are denormalized into the FTS5 index
	// at write-time, direct tags are already present. For inherited tags, at
	// index-build time we only store direct tags. So for tag-filter search we
	// additionally need to find files whose parent-folder (or ancestor) has
	// the requested tag. We handle this by augmenting search results.

	offset := (page - 1) * pageSize

	// --- 4. FTS5 primary search ------------------------------------------
	rows, total, err := s.searchRepo.Search(ctx, repository.SearchQuery{
		FTS5Query:      fts5Query,
		TagFilters:     tagFilters,
		ScopeFolderIDs: scopeFolderIDs,
		Offset:         offset,
		Limit:          pageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("fts5 search: %w", err)
	}

	// --- 5. LIKE fallback if FTS5 returned nothing -----------------------
	if total == 0 && hasFTS {
		rows, total, err = s.searchRepo.SearchWithLike(ctx, strings.ToLower(keyword), scopeFolderIDs, offset, pageSize)
		if err != nil {
			return nil, fmt.Errorf("like search fallback: %w", err)
		}
	}

	// --- 6. If tag filter active and FTS5 missed inherited tags, do
	//        supplementary search for files in folders that have those tags.
	if total == 0 && len(tagFilters) > 0 {
		supplementary, supTotal, supErr := s.searchByInheritedTags(ctx, tagFilters, fts5Query, scopeFolderIDs, offset, pageSize)
		if supErr != nil {
			return nil, fmt.Errorf("inherited tag search: %w", supErr)
		}
		if supTotal > 0 {
			rows = supplementary
			total = supTotal
		}
	}

	if total == 0 {
		return &SearchResult{
			Items:    []SearchResultItem{},
			Page:     page,
			PageSize: pageSize,
			Total:    0,
		}, nil
	}

	// --- 7. Hydrate results with metadata --------------------------------
	items, err := s.hydrateResults(ctx, rows)
	if err != nil {
		return nil, fmt.Errorf("hydrate results: %w", err)
	}

	return &SearchResult{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// ---------------------------------------------------------------------------
// Tag inheritance search
// ---------------------------------------------------------------------------

// searchByInheritedTags finds files that don't directly have the requested
// tags but inherit them from a parent/ancestor folder.
func (s *SearchService) searchByInheritedTags(
	ctx context.Context,
	tagFilters []string,
	fts5Query string,
	scopeFolderIDs []string,
	offset, limit int,
) ([]repository.SearchResultRow, int64, error) {
	// Find folders that have ALL of the requested tags directly.
	folderIDs, err := s.findFoldersWithAllTags(ctx, tagFilters)
	if err != nil {
		return nil, 0, err
	}
	if len(folderIDs) == 0 {
		return nil, 0, nil
	}

	// Expand to include descendant folders (files inherit ancestor tags).
	allFolderIDs := make(map[string]struct{})
	for _, fid := range folderIDs {
		allFolderIDs[fid] = struct{}{}
		descendants, err := s.searchRepo.GetDescendantFolderIDs(ctx, fid)
		if err != nil {
			return nil, 0, err
		}
		for _, did := range descendants {
			allFolderIDs[did] = struct{}{}
		}
	}

	// Intersect with scope if provided
	var searchScope []string
	if scopeFolderIDs != nil {
		scopeSet := make(map[string]struct{}, len(scopeFolderIDs))
		for _, id := range scopeFolderIDs {
			scopeSet[id] = struct{}{}
		}
		for id := range allFolderIDs {
			if _, ok := scopeSet[id]; ok {
				searchScope = append(searchScope, id)
			}
		}
	} else {
		for id := range allFolderIDs {
			searchScope = append(searchScope, id)
		}
	}

	if len(searchScope) == 0 {
		return nil, 0, nil
	}

	// If we also have a keyword, use FTS5 with folder scope.
	if fts5Query != "" {
		return s.searchRepo.Search(ctx, repository.SearchQuery{
			FTS5Query:      fts5Query,
			ScopeFolderIDs: searchScope,
			Offset:         offset,
			Limit:          limit,
		})
	}

	// Tag-only search: return files within these folders.
	return s.searchRepo.SearchWithLike(ctx, "", searchScope, offset, limit)
}

// findFoldersWithAllTags returns folder IDs that have ALL specified tags.
func (s *SearchService) findFoldersWithAllTags(ctx context.Context, tagFilters []string) ([]string, error) {
	if len(tagFilters) == 0 {
		return nil, nil
	}

	// Find tag entities matching the normalized names.
	tags, err := s.tagRepo.FindTagsByNormalizedNames(ctx, tagFilters)
	if err != nil {
		return nil, fmt.Errorf("find tags: %w", err)
	}
	if len(tags) < len(tagFilters) {
		// Not all requested tags exist → no folder can have all of them.
		return nil, nil
	}

	tagIDs := make([]string, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	// Find folders that have ALL of these tag IDs bound.
	folderIDs, err := s.searchRepo.FindFoldersWithAllTagIDs(ctx, tagIDs)
	if err != nil {
		return nil, fmt.Errorf("find folders with tags: %w", err)
	}
	return folderIDs, nil
}

// ---------------------------------------------------------------------------
// Result hydration
// ---------------------------------------------------------------------------

func (s *SearchService) hydrateResults(ctx context.Context, rows []repository.SearchResultRow) ([]SearchResultItem, error) {
	var fileIDs, folderIDs []string
	for _, row := range rows {
		switch row.EntityType {
		case "file":
			fileIDs = append(fileIDs, row.EntityID)
		case "folder":
			folderIDs = append(folderIDs, row.EntityID)
		}
	}

	// Load metadata
	fileMap := make(map[string]*fileHydrated)
	if len(fileIDs) > 0 {
		files, err := s.searchRepo.GetFilesByIDs(ctx, fileIDs)
		if err != nil {
			return nil, err
		}
		for i := range files {
			f := files[i]
			fileMap[f.ID] = &fileHydrated{file: &f}
		}
		tagMap, err := s.searchRepo.GetTagNamesByFileIDs(ctx, fileIDs)
		if err != nil {
			return nil, err
		}
		for id, tags := range tagMap {
			if h, ok := fileMap[id]; ok {
				h.tags = tags
			}
		}
	}

	folderMap := make(map[string]*folderHydrated)
	if len(folderIDs) > 0 {
		folders, err := s.searchRepo.GetFoldersByIDs(ctx, folderIDs)
		if err != nil {
			return nil, err
		}
		for i := range folders {
			f := folders[i]
			folderMap[f.ID] = &folderHydrated{folder: &f}
		}
		tagMap, err := s.searchRepo.GetTagNamesByFolderIDs(ctx, folderIDs)
		if err != nil {
			return nil, err
		}
		for id, tags := range tagMap {
			if h, ok := folderMap[id]; ok {
				h.tags = tags
			}
		}
	}

	// Assemble in original order
	items := make([]SearchResultItem, 0, len(rows))
	for _, row := range rows {
		switch row.EntityType {
		case "file":
			h, ok := fileMap[row.EntityID]
			if !ok {
				continue
			}
			tags := h.tags
			if tags == nil {
				tags = []string{}
			}
			t := h.file.CreatedAt
			items = append(items, SearchResultItem{
				EntityType:    "file",
				ID:            h.file.ID,
				Name:          h.file.Title,
				Tags:          tags,
				Size:          h.file.Size,
				DownloadCount: h.file.DownloadCount,
				UploadedAt:    &t,
			})
		case "folder":
			h, ok := folderMap[row.EntityID]
			if !ok {
				continue
			}
			tags := h.tags
			if tags == nil {
				tags = []string{}
			}
			items = append(items, SearchResultItem{
				EntityType: "folder",
				ID:         h.folder.ID,
				Name:       h.folder.Name,
				Tags:       tags,
			})
		}
	}

	return items, nil
}

type fileHydrated struct {
	file *model.File
	tags []string
}

type folderHydrated struct {
	folder *model.Folder
	tags   []string
}

// ---------------------------------------------------------------------------
// Index sync public API (called by other services after mutations)
// ---------------------------------------------------------------------------

// IndexFile updates the FTS5 index for a single file including its direct tags.
func (s *SearchService) IndexFile(ctx context.Context, fileID, title string) error {
	tagMap, err := s.searchRepo.GetTagNamesByFileIDs(ctx, []string{fileID})
	if err != nil {
		return fmt.Errorf("get file tags for indexing: %w", err)
	}
	return s.searchRepo.UpsertFileIndex(ctx, fileID, title, tagMap[fileID])
}

// IndexFolder updates the FTS5 index for a single folder.
func (s *SearchService) IndexFolder(ctx context.Context, folderID, name string) error {
	tagMap, err := s.searchRepo.GetTagNamesByFolderIDs(ctx, []string{folderID})
	if err != nil {
		return fmt.Errorf("get folder tags for indexing: %w", err)
	}
	return s.searchRepo.UpsertFolderIndex(ctx, folderID, name, tagMap[folderID])
}

// RemoveFromIndex removes an entity from the FTS5 index.
func (s *SearchService) RemoveFromIndex(ctx context.Context, entityType, entityID string) error {
	return s.searchRepo.RemoveIndex(ctx, entityType, entityID)
}

// RebuildAllIndexes rebuilds the full FTS5 index from scratch.
func (s *SearchService) RebuildAllIndexes(ctx context.Context) error {
	return s.searchRepo.RebuildAllIndexes(ctx)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func normalizeSearchPagination(page, pageSize int) (int, int, error) {
	if page == 0 {
		page = defaultSearchPage
	}
	if page < 1 {
		return 0, 0, ErrSearchInvalidInput
	}
	if pageSize == 0 {
		pageSize = defaultSearchPageSize
	}
	if pageSize < 1 || pageSize > maxSearchPageSize {
		return 0, 0, ErrSearchInvalidInput
	}
	return page, pageSize, nil
}
