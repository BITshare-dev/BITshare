package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
)

func TestImportLocalDirectoryCreatesMetadata(t *testing.T) {
	_, _, cookie, engine, db := newImportRouteEnv(t, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)

	recorder := importLocalDirectory(t, engine, cookie, importRoot)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var folderCount int64
	if err := db.Model(&model.Folder{}).Count(&folderCount).Error; err != nil {
		t.Fatalf("count folders failed: %v", err)
	}
	if folderCount != 2 {
		t.Fatalf("expected 2 folders, got %d", folderCount)
	}

	var fileCount int64
	if err := db.Model(&model.File{}).Count(&fileCount).Error; err != nil {
		t.Fatalf("count files failed: %v", err)
	}
	if fileCount != 2 {
		t.Fatalf("expected 2 files, got %d", fileCount)
	}

	var ignoredCount int64
	if err := db.Model(&model.File{}).Where("name = ?", ".DS_Store").Count(&ignoredCount).Error; err != nil {
		t.Fatalf("count ignored files failed: %v", err)
	}
	if ignoredCount != 0 {
		t.Fatalf("expected .DS_Store to be ignored, got %d records", ignoredCount)
	}

	var file model.File
	targetPath := filepath.Join(importRoot, "nested", "chapter1.txt")
	if err := db.Where("name = ?", filepath.Base(targetPath)).Take(&file).Error; err != nil {
		t.Fatalf("find imported file failed: %v", err)
	}
	var linkedSubmissionCount int64
	if err := db.Model(&model.Submission{}).Where("file_id = ?", file.ID).Count(&linkedSubmissionCount).Error; err != nil {
		t.Fatalf("count linked submissions failed: %v", err)
	}
	if linkedSubmissionCount != 0 {
		t.Fatalf("expected imported file to have no linked submissions, got %d", linkedSubmissionCount)
	}
}

func TestFolderTree(t *testing.T) {
	_, _, cookie, engine, db := newImportRouteEnv(t, adminAccess{
		username: "editor",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)

	importRecorder := importLocalDirectory(t, engine, cookie, importRoot)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("expected import status 200, got %d", importRecorder.Code)
	}

	var rootFolder model.Folder
	if err := db.Where("source_path = ?", importRoot).Take(&rootFolder).Error; err != nil {
		t.Fatalf("find root folder failed: %v", err)
	}

	treeRequest := httptest.NewRequest(http.MethodGet, "/api/admin/folders/tree", nil)
	treeRequest.AddCookie(cookie)
	treeRecorder := httptest.NewRecorder()
	engine.ServeHTTP(treeRecorder, treeRequest)
	if treeRecorder.Code != http.StatusOK {
		t.Fatalf("expected tree status 200, got %d body=%s", treeRecorder.Code, treeRecorder.Body.String())
	}

	var response struct {
		Items []struct {
			ID      string `json:"id"`
			Folders []struct {
				Name string `json:"name"`
			} `json:"folders"`
			Files []struct {
				OriginalName string `json:"original_name"`
			} `json:"files"`
		} `json:"items"`
	}
	if err := json.Unmarshal(treeRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tree response failed: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("expected 1 root folder, got %d", len(response.Items))
	}
	if len(response.Items[0].Folders) != 1 {
		t.Fatalf("expected 1 child folder, got %d", len(response.Items[0].Folders))
	}
	if len(response.Items[0].Files) != 1 {
		t.Fatalf("expected 1 root file, got %d", len(response.Items[0].Files))
	}
}

func TestImportDirectoryBrowser(t *testing.T) {
	_, _, cookie, engine, db := newImportRouteEnv(t, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	_ = db
	importRoot := createImportFixture(t)

	request := httptest.NewRequest(http.MethodGet, "/api/admin/imports/directories?path="+importRoot, nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		CurrentPath string `json:"current_path"`
		Items       []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.CurrentPath != importRoot {
		t.Fatalf("expected current path %q, got %q", importRoot, response.CurrentPath)
	}
	if len(response.Items) != 1 || response.Items[0].Name != "nested" {
		t.Fatalf("expected nested directory listing, got %+v", response.Items)
	}
}
