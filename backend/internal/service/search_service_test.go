package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

func TestSearchPrefersNameMatchesOverDescription(t *testing.T) {
	db := newTestSQLite(t)
	service := NewSearchService(repository.NewSearchRepository(db))

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	mustCreateSearchFile(t, db, model.File{
		ID:            "file-name-match",
		Name:          "logo_best.svg",
		Description:   "",
		Extension:     "svg",
		Size:          1024,
		DownloadCount: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	mustCreateSearchFile(t, db, model.File{
		ID:            "file-description-match",
		Name:          "notes.txt",
		Description:   "contains logo in description only",
		Extension:     "txt",
		Size:          2048,
		DownloadCount: 80,
		CreatedAt:     now.Add(1 * time.Hour),
		UpdatedAt:     now.Add(1 * time.Hour),
	})

	result, err := service.Search(context.Background(), SearchInput{
		Keyword:  "logo",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.Total != 2 {
		t.Fatalf("expected 2 results, got %d", result.Total)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].ID != "file-name-match" {
		t.Fatalf("expected name match first, got %q", result.Items[0].ID)
	}
}

func TestSearchRequiresAllTerms(t *testing.T) {
	db := newTestSQLite(t)
	service := NewSearchService(repository.NewSearchRepository(db))

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	mustCreateSearchFile(t, db, model.File{
		ID:        "macro-book",
		Name:      "高鸿业 宏观经济学.pdf",
		Extension: "pdf",
		Size:      2048,
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateSearchFile(t, db, model.File{
		ID:        "micro-book",
		Name:      "高鸿业 微观经济学.pdf",
		Extension: "pdf",
		Size:      2048,
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateSearchFolder(t, db, model.Folder{
		ID:          "macro-folder",
		Name:        "宏观专题",
		Description: "",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	result, err := service.Search(context.Background(), SearchInput{
		Keyword:  "高鸿业 宏观",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.Total != 1 {
		t.Fatalf("expected 1 result, got %d", result.Total)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "macro-book" {
		t.Fatalf("expected macro-book only, got %+v", result.Items)
	}
}

func TestSearchPrefersDirectFolderMatchesWithinScope(t *testing.T) {
	db := newTestSQLite(t)
	service := NewSearchService(repository.NewSearchRepository(db))

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	rootID := "folder-root"
	childID := "folder-child"

	mustCreateSearchFolder(t, db, model.Folder{
		ID:        rootID,
		Name:      "课程资料",
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateSearchFolder(t, db, model.Folder{
		ID:        childID,
		ParentID:  ptrString(rootID),
		Name:      "归档",
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateSearchFile(t, db, model.File{
		ID:        "direct-file",
		FolderID:  ptrString(rootID),
		Name:      "lecture-direct.pdf",
		Extension: "pdf",
		Size:      2048,
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateSearchFile(t, db, model.File{
		ID:        "nested-file",
		FolderID:  ptrString(childID),
		Name:      "lecture-nested.pdf",
		Extension: "pdf",
		Size:      2048,
		CreatedAt: now,
		UpdatedAt: now,
	})

	result, err := service.Search(context.Background(), SearchInput{
		Keyword:  "lecture",
		FolderID: rootID,
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.Total != 2 {
		t.Fatalf("expected 2 scoped results, got %d", result.Total)
	}
	if len(result.Items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(result.Items))
	}
	if result.Items[0].ID != "direct-file" {
		t.Fatalf("expected direct folder match first, got %q", result.Items[0].ID)
	}
}

func TestSearchEscapesLikeWildcards(t *testing.T) {
	db := newTestSQLite(t)
	service := NewSearchService(repository.NewSearchRepository(db))

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	mustCreateSearchFile(t, db, model.File{
		ID:        "plain-file",
		Name:      "ordinary.txt",
		Extension: "txt",
		Size:      1024,
		CreatedAt: now,
		UpdatedAt: now,
	})

	result, err := service.Search(context.Background(), SearchInput{
		Keyword:  "%",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.Total != 0 {
		t.Fatalf("expected 0 results for literal wildcard query, got %d", result.Total)
	}
}

func mustCreateSearchFile(t *testing.T, db *gorm.DB, file model.File) {
	t.Helper()
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file %q failed: %v", file.ID, err)
	}
}

func mustCreateSearchFolder(t *testing.T, db *gorm.DB, folder model.Folder) {
	t.Helper()
	if err := db.Create(&folder).Error; err != nil {
		t.Fatalf("create folder %q failed: %v", folder.ID, err)
	}
}

func ptrString(value string) *string {
	return &value
}
