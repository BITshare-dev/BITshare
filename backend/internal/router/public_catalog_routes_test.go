package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

func TestPublicHotFilesListsMostDownloadedFiles(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	folderID := createPublicTestFolder(t, db, "导入资料")
	createPublicTestFile(t, db, publicTestFile{
		title:         "公开文件",
		downloadCount: 7,
		size:          128,
	})
	createPublicTestFile(t, db, publicTestFile{
		title:    "目录内文件",
		folderID: &folderID,
		size:     256,
	})

	engine := New(db, cfg, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/files/hot?limit=10", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if len(response.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(response.Items))
	}

	if response.Items[0].Name != "公开文件.pdf" || response.Items[1].Name != "目录内文件.pdf" {
		t.Fatalf("unexpected hot files order: %+v", response.Items)
	}
}

func TestPublicFolderFilesSupportsFolderBrowsing(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	folderID := createPublicTestFolder(t, db, "课程资料")
	createPublicTestFile(t, db, publicTestFile{
		title:    "根目录文件",
		folderID: nil,
	})
	createPublicTestFile(t, db, publicTestFile{
		title:    "目录内文件",
		folderID: &folderID,
	})

	engine := New(db, cfg, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/folders/"+folderID+"/files", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if len(response.Items) != 1 || response.Items[0].Name != "目录内文件.pdf" {
		t.Fatalf("expected only folder item, got %+v", response.Items)
	}
}

func TestPublicLatestFilesReturnsNewestFirst(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	createPublicTestFile(t, db, publicTestFile{
		title:         "低下载",
		downloadCount: 1,
		createdAt:     time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC),
	})
	createPublicTestFile(t, db, publicTestFile{
		title:         "高下载",
		downloadCount: 20,
		createdAt:     time.Date(2026, 3, 11, 11, 0, 0, 0, time.UTC),
	})
	createPublicTestFile(t, db, publicTestFile{
		title:         "中下载",
		downloadCount: 10,
		createdAt:     time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC),
	})

	engine := New(db, cfg, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/files/latest?limit=2", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if len(response.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(response.Items))
	}
	if response.Items[0].Name != "中下载.pdf" || response.Items[1].Name != "高下载.pdf" {
		t.Fatalf("unexpected latest file order: %+v", response.Items)
	}
}

func TestPublicFolderFilesSupportsNameSort(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	folderID := createPublicTestFolder(t, db, "排序目录")
	createPublicTestFile(t, db, publicTestFile{title: "c-file", folderID: &folderID})
	createPublicTestFile(t, db, publicTestFile{title: "a-file", folderID: &folderID})
	createPublicTestFile(t, db, publicTestFile{title: "b-file", folderID: &folderID})

	engine := New(db, cfg, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/folders/"+folderID+"/files?sort=name_asc&page=1&page_size=10", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if len(response.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(response.Items))
	}
	if response.Items[0].Name != "a-file.pdf" || response.Items[1].Name != "b-file.pdf" || response.Items[2].Name != "c-file.pdf" {
		t.Fatalf("unexpected name order: %+v", response.Items)
	}
}

func TestPublicFoldersReturnsBreadcrumbs(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	rootID := createPublicTestFolderWithParent(t, db, publicTestFolder{
		name: "课程资料",
	})
	childID := createPublicTestFolderWithParent(t, db, publicTestFolder{
		name:        "高数",
		parentID:    &rootID,
		description: "高数简介",
	})

	engine := New(db, cfg, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/folders/"+childID, nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		FileCount   int64  `json:"file_count"`
		TotalSize   int64  `json:"total_size"`
		Breadcrumbs []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"breadcrumbs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.ID != childID {
		t.Fatalf("expected folder id %q, got %q", childID, response.ID)
	}
	if response.Description != "高数简介" {
		t.Fatalf("expected folder description, got %q", response.Description)
	}
	if len(response.Breadcrumbs) != 2 {
		t.Fatalf("expected 2 breadcrumbs, got %+v", response.Breadcrumbs)
	}
	if response.Breadcrumbs[0].Name != "课程资料" || response.Breadcrumbs[1].Name != "高数" {
		t.Fatalf("unexpected breadcrumbs: %+v", response.Breadcrumbs)
	}
}

func TestPublicFoldersReturnsAggregatedStats(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	rootID := createPublicTestFolderWithParent(t, db, publicTestFolder{
		name: "课程资料",
	})
	childID := createPublicTestFolderWithParent(t, db, publicTestFolder{
		name:     "讲义",
		parentID: &rootID,
	})
	createPublicTestFile(t, db, publicTestFile{
		title:         "根目录文件",
		folderID:      &rootID,
		downloadCount: 3,
		size:          128,
	})
	createPublicTestFile(t, db, publicTestFile{
		title:         "子目录文件",
		folderID:      &childID,
		downloadCount: 7,
		size:          256,
	})

	engine := New(db, cfg, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/folders", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			ID            string `json:"id"`
			FileCount     int64  `json:"file_count"`
			DownloadCount int64  `json:"download_count"`
			TotalSize     int64  `json:"total_size"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if len(response.Items) != 1 {
		t.Fatalf("expected 1 root folder, got %d", len(response.Items))
	}
	if response.Items[0].ID != rootID {
		t.Fatalf("unexpected folder id %q", response.Items[0].ID)
	}
	if response.Items[0].FileCount != 2 {
		t.Fatalf("expected file_count 2, got %d", response.Items[0].FileCount)
	}
	if response.Items[0].DownloadCount != 10 {
		t.Fatalf("expected download_count 10, got %d", response.Items[0].DownloadCount)
	}
	if response.Items[0].TotalSize != 384 {
		t.Fatalf("expected total_size 384, got %d", response.Items[0].TotalSize)
	}
}

type publicTestFile struct {
	title         string
	folderID      *string
	downloadCount int64
	size          int64
	createdAt     time.Time
}

func createPublicTestFile(t *testing.T, db *gorm.DB, input publicTestFile) *model.File {
	t.Helper()

	fileID := mustNewID(t)
	createdAt := input.createdAt
	if createdAt.IsZero() {
		createdAt = time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)
	}

	file := &model.File{
		ID:            fileID,
		FolderID:      input.folderID,
		Name:          input.title + ".pdf",
		Extension:     "pdf",
		MimeType:      "application/pdf",
		Size:          input.size,
		DownloadCount: input.downloadCount,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create public test file failed: %v", err)
	}

	return file
}

type publicTestFolder struct {
	name        string
	parentID    *string
	description string
}

func createPublicTestFolder(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	return createPublicTestFolderWithParent(t, db, publicTestFolder{name: name})
}

func createPublicTestFolderWithParent(t *testing.T, db *gorm.DB, input publicTestFolder) string {
	t.Helper()

	folderID := mustNewID(t)
	sourcePath := filepath.Join(t.TempDir(), input.name)
	if err := os.MkdirAll(sourcePath, 0o755); err != nil {
		t.Fatalf("create public test folder path failed: %v", err)
	}
	folder := &model.Folder{
		ID:          folderID,
		ParentID:    input.parentID,
		Name:        input.name,
		Description: input.description,
		SourcePath:  &sourcePath,
		CreatedAt:   time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC),
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create public test folder failed: %v", err)
	}

	return folderID
}

func mustNewID(t *testing.T) string {
	t.Helper()

	id, err := identity.NewID()
	if err != nil {
		t.Fatalf("generate id failed: %v", err)
	}

	return id
}
