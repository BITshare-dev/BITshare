package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"openshare/backend/internal/model"
)

// SearchRepository handles candidate recall over the ordinary resource tables.
type SearchRepository struct {
	db *gorm.DB
}

func NewSearchRepository(db *gorm.DB) *SearchRepository {
	return &SearchRepository{db: db}
}

// SearchCandidateQuery encapsulates parameters for LIKE-based candidate recall.
type SearchCandidateQuery struct {
	FullQuery      string
	Terms          []string
	ScopeFolderIDs []string
	Limit          int
}

// SearchCandidate is a hydrated search row used for application-side ranking.
type SearchCandidate struct {
	EntityType    string
	ID            string
	Name          string
	Description   string
	Extension     string
	Size          int64
	DownloadCount int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	FolderID      *string
	ParentID      *string
}

// SearchCandidates recalls managed file and folder candidates using parameterized
// LIKE matching. It returns raw candidates for service-layer ranking.
func (r *SearchRepository) SearchCandidates(ctx context.Context, query SearchCandidateQuery) ([]SearchCandidate, int64, error) {
	files, fileTotal, err := r.searchFilesForCandidates(ctx, query)
	if err != nil {
		return nil, 0, err
	}

	folders, folderTotal, err := r.searchFoldersForCandidates(ctx, query)
	if err != nil {
		return nil, 0, err
	}

	candidates := make([]SearchCandidate, 0, len(files)+len(folders))
	for i := range files {
		file := files[i]
		candidates = append(candidates, SearchCandidate{
			EntityType:    "file",
			ID:            file.ID,
			Name:          file.Name,
			Description:   file.Description,
			Extension:     file.Extension,
			Size:          file.Size,
			DownloadCount: file.DownloadCount,
			CreatedAt:     file.CreatedAt,
			UpdatedAt:     file.UpdatedAt,
			FolderID:      file.FolderID,
		})
	}

	for i := range folders {
		folder := folders[i]
		candidates = append(candidates, SearchCandidate{
			EntityType:    "folder",
			ID:            folder.ID,
			Name:          folder.Name,
			Description:   folder.Description,
			DownloadCount: folder.DownloadCount,
			CreatedAt:     folder.CreatedAt,
			UpdatedAt:     folder.UpdatedAt,
			ParentID:      folder.ParentID,
		})
	}

	return candidates, fileTotal + folderTotal, nil
}

func (r *SearchRepository) searchFilesForCandidates(ctx context.Context, query SearchCandidateQuery) ([]model.File, int64, error) {
	db := r.db.WithContext(ctx).
		Model(&model.File{})

	if query.ScopeFolderIDs != nil {
		db = db.Where("folder_id IN ?", query.ScopeFolderIDs)
	}

	db = applySearchTermFilters(db, []string{"name", "description"}, query.Terms)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count file search candidates: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	var files []model.File
	findDB := applyCandidateOrder(db, []string{"name"}, "description", "download_count", "updated_at", query.FullQuery)
	if query.Limit > 0 {
		findDB = findDB.Limit(query.Limit)
	}
	if err := findDB.Find(&files).Error; err != nil {
		return nil, 0, fmt.Errorf("load file search candidates: %w", err)
	}

	return files, total, nil
}

func (r *SearchRepository) searchFoldersForCandidates(ctx context.Context, query SearchCandidateQuery) ([]model.Folder, int64, error) {
	db := r.db.WithContext(ctx).
		Model(&model.Folder{})

	if query.ScopeFolderIDs != nil {
		db = db.Where("id IN ?", query.ScopeFolderIDs)
	}

	db = applySearchTermFilters(db, []string{"name", "description"}, query.Terms)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count folder search candidates: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	var folders []model.Folder
	findDB := applyCandidateOrder(db, []string{"name"}, "description", "download_count", "updated_at", query.FullQuery)
	if query.Limit > 0 {
		findDB = findDB.Limit(query.Limit)
	}
	if err := findDB.Find(&folders).Error; err != nil {
		return nil, 0, fmt.Errorf("load folder search candidates: %w", err)
	}

	return folders, total, nil
}

func applySearchTermFilters(db *gorm.DB, fields []string, terms []string) *gorm.DB {
	for _, term := range terms {
		if strings.TrimSpace(term) == "" {
			continue
		}

		pattern := containsLikePattern(term)
		conditions := make([]string, 0, len(fields))
		args := make([]any, 0, len(fields))
		for _, field := range fields {
			conditions = append(conditions, fmt.Sprintf("LOWER(%s) LIKE ? ESCAPE '\\'", field))
			args = append(args, pattern)
		}
		db = db.Where("("+strings.Join(conditions, " OR ")+")", args...)
	}

	return db
}

func applyCandidateOrder(db *gorm.DB, primaryFields []string, descriptionField, downloadField, updatedField, fullQuery string) *gorm.DB {
	if strings.TrimSpace(fullQuery) == "" {
		return db.Order(downloadField + " DESC").Order(updatedField + " DESC")
	}

	equalConditions := make([]string, 0, len(primaryFields))
	prefixConditions := make([]string, 0, len(primaryFields))
	containsConditions := make([]string, 0, len(primaryFields))
	args := make([]any, 0, len(primaryFields)*3+1)

	for range primaryFields {
		args = append(args, fullQuery)
	}
	for _, field := range primaryFields {
		equalConditions = append(equalConditions, fmt.Sprintf("LOWER(%s) = ?", field))
	}

	prefixPattern := prefixLikePattern(fullQuery)
	for _, field := range primaryFields {
		prefixConditions = append(prefixConditions, fmt.Sprintf("LOWER(%s) LIKE ? ESCAPE '\\'", field))
		args = append(args, prefixPattern)
	}

	containsPattern := containsLikePattern(fullQuery)
	for _, field := range primaryFields {
		containsConditions = append(containsConditions, fmt.Sprintf("LOWER(%s) LIKE ? ESCAPE '\\'", field))
		args = append(args, containsPattern)
	}

	descriptionPattern := containsLikePattern(fullQuery)
	args = append(args, descriptionPattern)

	sql := fmt.Sprintf(`
CASE
	WHEN %s THEN 0
	WHEN %s THEN 1
	WHEN %s THEN 2
	WHEN LOWER(%s) LIKE ? ESCAPE '\' THEN 3
	ELSE 4
END
`, strings.Join(equalConditions, " OR "), strings.Join(prefixConditions, " OR "), strings.Join(containsConditions, " OR "), descriptionField)

	return db.
		Order(clause.Expr{SQL: sql, Vars: args}).
		Order(downloadField + " DESC").
		Order(updatedField + " DESC")
}

// GetFilesByIDs loads file metadata for a list of IDs, preserving order.
func (r *SearchRepository) GetFilesByIDs(ctx context.Context, ids []string) ([]model.File, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var files []model.File
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&files).Error; err != nil {
		return nil, fmt.Errorf("get files by ids: %w", err)
	}
	return files, nil
}

// GetFoldersByIDs loads folder metadata for a list of IDs.
func (r *SearchRepository) GetFoldersByIDs(ctx context.Context, ids []string) ([]model.Folder, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var folders []model.Folder
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&folders).Error; err != nil {
		return nil, fmt.Errorf("get folders by ids: %w", err)
	}
	return folders, nil
}

// GetDescendantFolderIDs returns the given folderID plus all its descendants.
func (r *SearchRepository) GetDescendantFolderIDs(ctx context.Context, folderID string) ([]string, error) {
	result := []string{folderID}
	queue := []string{folderID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		var childIDs []string
		if err := r.db.WithContext(ctx).
			Model(&model.Folder{}).
			Where("parent_id = ?", current).
			Pluck("id", &childIDs).Error; err != nil {
			return nil, fmt.Errorf("get child folders: %w", err)
		}
		result = append(result, childIDs...)
		queue = append(queue, childIDs...)
	}
	return result, nil
}

// escapeLike escapes LIKE special characters.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func containsLikePattern(s string) string {
	return "%" + escapeLike(strings.ToLower(strings.TrimSpace(s))) + "%"
}

func prefixLikePattern(s string) string {
	return escapeLike(strings.ToLower(strings.TrimSpace(s))) + "%"
}
