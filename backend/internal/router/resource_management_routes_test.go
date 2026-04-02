package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func TestAdminUpdateFolderAllowsSameNameInDifferentDirectories(t *testing.T) {
	db, cookie, engine := newResourceManagementRouteEnv(t)

	rootFolderID := createPublicTestFolder(t, db, "课程资料")
	otherRootFolderID := createPublicTestFolder(t, db, "其他资料")
	folderID := createPublicTestFolderWithParent(t, db, publicTestFolder{
		name:     "半导体物理资料",
		parentID: &rootFolderID,
	})
	_ = createPublicTestFolderWithParent(t, db, publicTestFolder{
		name:     "半导体物理",
		parentID: &otherRootFolderID,
	})

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources/folders/"+folderID,
		bytes.NewBufferString(`{"name":"半导体物理","description":"更新后的目录"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updated model.Folder
	if err := db.Where("id = ?", folderID).Take(&updated).Error; err != nil {
		t.Fatalf("reload folder failed: %v", err)
	}
	if updated.Name != "半导体物理" {
		t.Fatalf("expected updated folder name, got %q", updated.Name)
	}
	if updated.SourcePath == nil || filepath.Base(*updated.SourcePath) != "半导体物理" {
		t.Fatalf("expected folder source path renamed, got %+v", updated.SourcePath)
	}
}

func TestAdminUpdateFolderRejectsSiblingFileNameConflict(t *testing.T) {
	db, cookie, engine := newResourceManagementRouteEnv(t)

	rootFolderID := createPublicTestFolder(t, db, "课程资料")
	folderID := createPublicTestFolderWithParent(t, db, publicTestFolder{
		name:     "半导体物理资料",
		parentID: &rootFolderID,
	})
	createManagedTestFile(t, db, &rootFolderID, "半导体物理")

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources/folders/"+folderID,
		bytes.NewBufferString(`{"name":"半导体物理","description":""}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminUpdateFileRejectsSiblingFileNameConflict(t *testing.T) {
	db, cookie, engine := newResourceManagementRouteEnv(t)

	rootFolderID := createPublicTestFolder(t, db, "课程资料")
	fileID := createManagedTestFile(t, db, &rootFolderID, "半导体物理资料.pdf")
	createManagedTestFile(t, db, &rootFolderID, "半导体物理.pdf")

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources/files/"+fileID,
		bytes.NewBufferString(`{"name":"半导体物理.pdf","description":""}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminUpdateFileAllowsSameNameInDifferentDirectory(t *testing.T) {
	db, cookie, engine := newResourceManagementRouteEnv(t)

	rootFolderID := createPublicTestFolder(t, db, "课程资料")
	otherRootFolderID := createPublicTestFolder(t, db, "其他资料")
	fileID := createManagedTestFile(t, db, &rootFolderID, "原始资料.pdf")
	createManagedTestFile(t, db, &otherRootFolderID, "半导体物理.pdf")

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources/files/"+fileID,
		bytes.NewBufferString(`{"name":"半导体物理.pdf","description":"更新后描述"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updated model.File
	if err := db.Where("id = ?", fileID).Take(&updated).Error; err != nil {
		t.Fatalf("reload file failed: %v", err)
	}
	if updated.Name != "半导体物理.pdf" {
		t.Fatalf("expected file name updated, got %q", updated.Name)
	}
}

func TestAdminUpdateFileRejectsSiblingNameThatDiffersOnlyByCase(t *testing.T) {
	db, cookie, engine := newResourceManagementRouteEnv(t)

	rootFolderID := createPublicTestFolder(t, db, "课程资料")
	fileID := createManagedTestFile(t, db, &rootFolderID, "Original.pdf")
	createManagedTestFile(t, db, &rootFolderID, "Semiconductor.pdf")

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources/files/"+fileID,
		bytes.NewBufferString(`{"name":"semiconductor.pdf","description":"更新后描述"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func newResourceManagementRouteEnv(t *testing.T) (*gorm.DB, *http.Cookie, http.Handler) {
	t.Helper()

	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "resource-admin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionResourceModeration,
		},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	return db, &http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"}, engine
}

func createManagedTestFile(t *testing.T, db *gorm.DB, folderID *string, originalName string) string {
	t.Helper()

	fileID := mustNewID(t)
	extension := filepath.Ext(originalName)
	folderPath := t.TempDir()
	if folderID != nil {
		var folder model.Folder
		if err := db.Where("id = ?", *folderID).Take(&folder).Error; err != nil {
			t.Fatalf("load managed test folder failed: %v", err)
		}
		if folder.SourcePath != nil && strings.TrimSpace(*folder.SourcePath) != "" {
			folderPath = *folder.SourcePath
		}
	}
	if err := os.MkdirAll(folderPath, 0o755); err != nil {
		t.Fatalf("ensure managed test folder path failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folderPath, originalName), []byte("test file"), 0o644); err != nil {
		t.Fatalf("write managed test file failed: %v", err)
	}

	now := time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC)
	file := &model.File{
		ID:            fileID,
		FolderID:      folderID,
		Name:          originalName,
		Description:   "",
		Extension:     extension,
		MimeType:      "application/octet-stream",
		Size:          int64(len("test file")),
		DownloadCount: 0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create managed test file failed: %v", err)
	}

	return fileID
}
