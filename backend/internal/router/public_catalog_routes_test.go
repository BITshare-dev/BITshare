package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

func TestPublicFilesListsOnlyActiveRootFiles(t *testing.T) {
	db := newRouterTestDB(t)
	rootActive := createPublicTestFile(t, db, publicTestFile{
		title:         "公开文件",
		status:        model.ResourceStatusActive,
		downloadCount: 7,
		size:          128,
	})
	createPublicTestFile(t, db, publicTestFile{
		title:  "下架文件",
		status: model.ResourceStatusOffline,
	})
	addTagsToFile(t, db, rootActive.ID, "数学", "物理")

	engine := New(db, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/files", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Title string   `json:"title"`
			Tags  []string `json:"tags"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if response.Total != 1 {
		t.Fatalf("expected total 1, got %d", response.Total)
	}
	if len(response.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(response.Items))
	}
	if response.Items[0].Title != "公开文件" {
		t.Fatalf("unexpected title %q", response.Items[0].Title)
	}
	if len(response.Items[0].Tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", response.Items[0].Tags)
	}
}

func TestPublicFilesSupportsFolderBrowsing(t *testing.T) {
	db := newRouterTestDB(t)
	folderID := createPublicTestFolder(t, db, "课程资料")
	createPublicTestFile(t, db, publicTestFile{
		title:    "根目录文件",
		status:   model.ResourceStatusActive,
		folderID: nil,
	})
	createPublicTestFile(t, db, publicTestFile{
		title:    "目录内文件",
		status:   model.ResourceStatusActive,
		folderID: &folderID,
	})

	engine := New(db, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/files?folder_id="+folderID, nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if len(response.Items) != 1 || response.Items[0].Title != "目录内文件" {
		t.Fatalf("expected only folder item, got %+v", response.Items)
	}
}

func TestPublicFilesSupportsPaginationAndSort(t *testing.T) {
	db := newRouterTestDB(t)
	createPublicTestFile(t, db, publicTestFile{
		title:         "低下载",
		status:        model.ResourceStatusActive,
		downloadCount: 1,
		createdAt:     time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC),
	})
	createPublicTestFile(t, db, publicTestFile{
		title:         "高下载",
		status:        model.ResourceStatusActive,
		downloadCount: 20,
		createdAt:     time.Date(2026, 3, 11, 11, 0, 0, 0, time.UTC),
	})
	createPublicTestFile(t, db, publicTestFile{
		title:         "中下载",
		status:        model.ResourceStatusActive,
		downloadCount: 10,
		createdAt:     time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC),
	})

	engine := New(db, newRouterSessionManager(db))
	request := httptest.NewRequest(http.MethodGet, "/api/public/files?sort=download_count_desc&page=1&page_size=2", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Total    int `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if response.Page != 1 || response.PageSize != 2 || response.Total != 3 {
		t.Fatalf("unexpected pagination metadata: %+v", response)
	}
	if len(response.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(response.Items))
	}
	if response.Items[0].Title != "高下载" || response.Items[1].Title != "中下载" {
		t.Fatalf("unexpected sorted order: %+v", response.Items)
	}
}

type publicTestFile struct {
	title         string
	status        model.ResourceStatus
	folderID      *string
	downloadCount int64
	size          int64
	createdAt     time.Time
}

func createPublicTestFile(t *testing.T, db *gorm.DB, input publicTestFile) *model.File {
	t.Helper()

	fileID := mustNewID(t)
	storedName := mustNewID(t) + ".bin"
	createdAt := input.createdAt
	if createdAt.IsZero() {
		createdAt = time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)
	}

	file := &model.File{
		ID:            fileID,
		FolderID:      input.folderID,
		Title:         input.title,
		OriginalName:  input.title + ".pdf",
		StoredName:    storedName,
		Extension:     ".pdf",
		MimeType:      "application/pdf",
		Size:          input.size,
		DiskPath:      "/data/openshare/repository/" + storedName,
		Status:        input.status,
		DownloadCount: input.downloadCount,
		UploaderIP:    "127.0.0.1",
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create public test file failed: %v", err)
	}

	return file
}

func createPublicTestFolder(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()

	folderID := mustNewID(t)
	folder := &model.Folder{
		ID:        folderID,
		Name:      name,
		Status:    model.ResourceStatusActive,
		CreatedAt: time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC),
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create public test folder failed: %v", err)
	}

	return folderID
}

func addTagsToFile(t *testing.T, db *gorm.DB, fileID string, names ...string) {
	t.Helper()

	for _, name := range names {
		tagID := mustNewID(t)
		tag := &model.Tag{
			ID:             tagID,
			Name:           name,
			NameNormalized: name,
			CreatedAt:      time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC),
		}
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag failed: %v", err)
		}

		fileTag := &model.FileTag{
			ID:        mustNewID(t),
			FileID:    fileID,
			TagID:     tagID,
			CreatedAt: time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC),
		}
		if err := db.Create(fileTag).Error; err != nil {
			t.Fatalf("create file tag failed: %v", err)
		}
	}
}

func mustNewID(t *testing.T) string {
	t.Helper()

	id, err := identity.NewID()
	if err != nil {
		t.Fatalf("generate id failed: %v", err)
	}

	return id
}
