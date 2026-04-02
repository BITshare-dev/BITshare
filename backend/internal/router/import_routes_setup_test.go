package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
	"openshare/backend/internal/model"
)

func newImportRouteEnv(t *testing.T, access adminAccess) (*model.Admin, string, *http.Cookie, http.Handler, *gorm.DB) {
	t.Helper()

	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, access)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	cookie := &http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"}
	return admin, cookieValue, cookie, engine, db
}

func importLocalDirectory(t *testing.T, engine http.Handler, cookie *http.Cookie, rootPath string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+rootPath+`"}`))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func createImportFixture(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "import-root")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatalf("create import fixture dirs failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "root.pdf"), []byte("root file"), 0o644); err != nil {
		t.Fatalf("write root fixture file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".DS_Store"), []byte("mac metadata"), 0o644); err != nil {
		t.Fatalf("write root ds_store fixture file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "chapter1.txt"), []byte("chapter one"), 0o644); err != nil {
		t.Fatalf("write nested fixture file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", ".DS_Store"), []byte("nested mac metadata"), 0o644); err != nil {
		t.Fatalf("write nested ds_store fixture file failed: %v", err)
	}

	return root
}
