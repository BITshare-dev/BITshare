package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

func TestImportLocalDirectoryCreatesMetadata(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"root_path":"` + importRoot + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

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
	if err := db.Model(&model.File{}).Where("original_name = ?", ".DS_Store").Count(&ignoredCount).Error; err != nil {
		t.Fatalf("count ignored files failed: %v", err)
	}
	if ignoredCount != 0 {
		t.Fatalf("expected .DS_Store to be ignored, got %d records", ignoredCount)
	}

	var file model.File
	targetPath := filepath.Join(importRoot, "nested", "chapter1.txt")
	if err := db.Where("disk_path = ?", targetPath).Take(&file).Error; err != nil {
		t.Fatalf("find imported file failed: %v", err)
	}
	if file.Status != model.ResourceStatusActive {
		t.Fatalf("expected imported file active, got %q", file.Status)
	}
	if file.SubmissionID != nil {
		t.Fatal("expected imported file to have nil submission_id")
	}
}

func TestImportLocalDirectoryRejectsDuplicateAndChildDirectory(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+importRoot+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	duplicateRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+importRoot+`"}`))
	duplicateRequest.Header.Set("Content-Type", "application/json")
	duplicateRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	duplicateRecorder := httptest.NewRecorder()
	engine.ServeHTTP(duplicateRecorder, duplicateRequest)
	if duplicateRecorder.Code != http.StatusConflict {
		t.Fatalf("expected duplicate import status 409, got %d body=%s", duplicateRecorder.Code, duplicateRecorder.Body.String())
	}

	childPath := filepath.Join(importRoot, "nested")
	childRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+childPath+`"}`))
	childRequest.Header.Set("Content-Type", "application/json")
	childRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	childRecorder := httptest.NewRecorder()
	engine.ServeHTTP(childRecorder, childRequest)
	if childRecorder.Code != http.StatusConflict {
		t.Fatalf("expected child import status 409, got %d body=%s", childRecorder.Code, childRecorder.Body.String())
	}
}

func TestImportLocalDirectoryRejectsParentDirectoryOfManagedRoot(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)
	childPath := filepath.Join(importRoot, "nested")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	childRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+childPath+`"}`))
	childRequest.Header.Set("Content-Type", "application/json")
	childRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	childRecorder := httptest.NewRecorder()
	engine.ServeHTTP(childRecorder, childRequest)
	if childRecorder.Code != http.StatusOK {
		t.Fatalf("expected child import status 200, got %d body=%s", childRecorder.Code, childRecorder.Body.String())
	}

	parentRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+importRoot+`"}`))
	parentRequest.Header.Set("Content-Type", "application/json")
	parentRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	parentRecorder := httptest.NewRecorder()
	engine.ServeHTTP(parentRecorder, parentRequest)
	if parentRecorder.Code != http.StatusConflict {
		t.Fatalf("expected parent import status 409, got %d body=%s", parentRecorder.Code, parentRecorder.Body.String())
	}
}

func TestFolderTree(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "editor",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	importRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+importRoot+`"}`))
	importRequest.Header.Set("Content-Type", "application/json")
	importRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	importRecorder := httptest.NewRecorder()
	engine.ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("expected import status 200, got %d", importRecorder.Code)
	}

	var rootFolder model.Folder
	if err := db.Where("source_path = ?", importRoot).Take(&rootFolder).Error; err != nil {
		t.Fatalf("find root folder failed: %v", err)
	}

	treeRequest := httptest.NewRequest(http.MethodGet, "/api/admin/folders/tree", nil)
	treeRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
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
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/imports/directories?path="+importRoot, nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
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

func TestDeleteManagedDirectoryRequiresSuperAdminPasswordAndCleansData(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "superadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleSuperAdmin),
	})
	importRoot := createImportFixture(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	importRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+importRoot+`"}`))
	importRequest.Header.Set("Content-Type", "application/json")
	importRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	importRecorder := httptest.NewRecorder()
	engine.ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("expected import status 200, got %d body=%s", importRecorder.Code, importRecorder.Body.String())
	}

	var rootFolder model.Folder
	if err := db.Where("source_path = ?", importRoot).Take(&rootFolder).Error; err != nil {
		t.Fatalf("find root folder failed: %v", err)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/admin/imports/local/"+rootFolder.ID, bytes.NewBufferString(`{"password":"s3cret-pass"}`))
	deleteRequest.Header.Set("Content-Type", "application/json")
	deleteRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	deleteRecorder := httptest.NewRecorder()
	engine.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d body=%s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	var folderCount int64
	if err := db.Model(&model.Folder{}).Count(&folderCount).Error; err != nil {
		t.Fatalf("count folders failed: %v", err)
	}
	if folderCount != 0 {
		t.Fatalf("expected all imported folders deleted, got %d", folderCount)
	}

	var fileCount int64
	if err := db.Model(&model.File{}).Count(&fileCount).Error; err != nil {
		t.Fatalf("count files failed: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected all imported files deleted, got %d", fileCount)
	}
}

func TestRescanManagedDirectoryMirrorsFilesystemAndPreservesHistoricalDownloadStats(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "superadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleSuperAdmin),
	})
	importRoot := createImportFixture(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	importRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local", bytes.NewBufferString(`{"root_path":"`+importRoot+`"}`))
	importRequest.Header.Set("Content-Type", "application/json")
	importRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	importRecorder := httptest.NewRecorder()
	engine.ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("expected import status 200, got %d body=%s", importRecorder.Code, importRecorder.Body.String())
	}

	var rootFolder model.Folder
	if err := db.Where("source_path = ?", importRoot).Take(&rootFolder).Error; err != nil {
		t.Fatalf("find root folder failed: %v", err)
	}

	renamedSourcePath := filepath.Join(importRoot, "nested", "chapter1.txt")
	var downloadedFile model.File
	if err := db.Where("disk_path = ?", renamedSourcePath).Take(&downloadedFile).Error; err != nil {
		t.Fatalf("find file for download failed: %v", err)
	}
	if err := repository.NewPublicDownloadRepository(db).IncrementDownloadCount(t.Context(), downloadedFile.ID); err != nil {
		t.Fatalf("increment download failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(importRoot, "root.pdf"), []byte("root file with larger contents"), 0o644); err != nil {
		t.Fatalf("rewrite root fixture file failed: %v", err)
	}
	renamedPath := filepath.Join(importRoot, "nested", "chapter-renamed.txt")
	if err := os.Rename(renamedSourcePath, renamedPath); err != nil {
		t.Fatalf("rename imported file failed: %v", err)
	}
	newDir := filepath.Join(importRoot, "nested", "newdir")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("create new directory failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "extra.md"), []byte("extra content"), 0o644); err != nil {
		t.Fatalf("write extra file failed: %v", err)
	}

	rescanRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local/"+rootFolder.ID+"/rescan", nil)
	rescanRequest.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	rescanRecorder := httptest.NewRecorder()
	engine.ServeHTTP(rescanRecorder, rescanRequest)
	if rescanRecorder.Code != http.StatusOK {
		t.Fatalf("expected rescan status 200, got %d body=%s", rescanRecorder.Code, rescanRecorder.Body.String())
	}

	var rescanResult struct {
		AddedFolders   int `json:"added_folders"`
		AddedFiles     int `json:"added_files"`
		UpdatedFiles   int `json:"updated_files"`
		DeletedFiles   int `json:"deleted_files"`
		DeletedFolders int `json:"deleted_folders"`
	}
	if err := json.Unmarshal(rescanRecorder.Body.Bytes(), &rescanResult); err != nil {
		t.Fatalf("decode rescan response failed: %v", err)
	}
	if rescanResult.AddedFolders != 1 || rescanResult.AddedFiles != 2 || rescanResult.UpdatedFiles != 1 || rescanResult.DeletedFiles != 1 || rescanResult.DeletedFolders != 0 {
		t.Fatalf("unexpected rescan result: %+v", rescanResult)
	}

	var missingCount int64
	if err := db.Model(&model.File{}).Where("disk_path = ?", renamedSourcePath).Count(&missingCount).Error; err != nil {
		t.Fatalf("count deleted file failed: %v", err)
	}
	if missingCount != 0 {
		t.Fatalf("expected renamed source file removed from db, got %d rows", missingCount)
	}

	var renamedFile model.File
	if err := db.Where("disk_path = ?", renamedPath).Take(&renamedFile).Error; err != nil {
		t.Fatalf("find renamed file failed: %v", err)
	}
	if renamedFile.DownloadCount != 0 {
		t.Fatalf("expected renamed file download count reset, got %d", renamedFile.DownloadCount)
	}

	var updatedRootFile model.File
	if err := db.Where("disk_path = ?", filepath.Join(importRoot, "root.pdf")).Take(&updatedRootFile).Error; err != nil {
		t.Fatalf("find updated root file failed: %v", err)
	}
	if updatedRootFile.Size <= int64(len("root file")) {
		t.Fatalf("expected root file size updated, got %d", updatedRootFile.Size)
	}

	var extraFile model.File
	if err := db.Where("disk_path = ?", filepath.Join(newDir, "extra.md")).Take(&extraFile).Error; err != nil {
		t.Fatalf("find extra file failed: %v", err)
	}
	if extraFile.SourcePath == nil || *extraFile.SourcePath != filepath.Join(newDir, "extra.md") {
		t.Fatalf("expected extra file source_path tracked, got %+v", extraFile.SourcePath)
	}

	if err := db.Where("id = ?", rootFolder.ID).Take(&rootFolder).Error; err != nil {
		t.Fatalf("reload root folder failed: %v", err)
	}
	if rootFolder.DownloadCount != 0 {
		t.Fatalf("expected current root folder download count reset to active resources, got %d", rootFolder.DownloadCount)
	}

	var systemStats model.SystemStat
	if err := db.Where("key = ?", model.GlobalSystemStatsKey).Take(&systemStats).Error; err != nil {
		t.Fatalf("find system stats failed: %v", err)
	}
	if systemStats.TotalDownloads != 1 {
		t.Fatalf("expected historical total downloads preserved, got %d", systemStats.TotalDownloads)
	}
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
