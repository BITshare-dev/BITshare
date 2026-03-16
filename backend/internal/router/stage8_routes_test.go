package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func TestAnnouncementLifecycleAndPublicVisibility(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "ann-admin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionManageAnnouncements},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	body := bytes.NewBufferString(`{"title":"维护通知","content":"今晚 10 点维护","status":"published"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/announcements", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create announcement: expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create announcement response: %v", err)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/api/public/announcements", nil)
	publicRec := httptest.NewRecorder()
	engine.ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK {
		t.Fatalf("list public announcements: expected 200, got %d", publicRec.Code)
	}
	var publicResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(publicRec.Body.Bytes(), &publicResp); err != nil {
		t.Fatalf("decode public announcement response: %v", err)
	}
	if len(publicResp.Items) != 1 || publicResp.Items[0].ID != created.ID {
		t.Fatalf("expected published announcement to be public, got %+v", publicResp.Items)
	}

	updateReq := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/announcements/"+created.ID,
		bytes.NewBufferString(`{"title":"维护通知","content":"今晚 10 点维护","status":"hidden"}`),
	)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateRec := httptest.NewRecorder()
	engine.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("hide announcement: expected 200, got %d, body=%s", updateRec.Code, updateRec.Body.String())
	}

	publicRec = httptest.NewRecorder()
	engine.ServeHTTP(publicRec, publicReq)
	if err := json.Unmarshal(publicRec.Body.Bytes(), &publicResp); err != nil {
		t.Fatalf("decode hidden public announcement response: %v", err)
	}
	if len(publicResp.Items) != 0 {
		t.Fatalf("expected hidden announcement to disappear from public list, got %+v", publicResp.Items)
	}
}

func TestSuperAdminCanManageAdminsAndNormalAdminCannot(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	superAdmin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "plain-admin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	superCookie := mustCreateSession(t, manager, superAdmin)
	adminCookie := mustCreateSession(t, manager, admin)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/admins",
		bytes.NewBufferString(`{"permissions":["manage_tags"]}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(superCookie)
	createRec := httptest.NewRecorder()
	engine.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("super admin create admin: expected 201, got %d, body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/admins", nil)
	listReq.AddCookie(adminCookie)
	listRec := httptest.NewRecorder()
	engine.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("normal admin should be allowed to view admin list, got %d body=%s", listRec.Code, listRec.Body.String())
	}

	forbiddenReq := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/admins",
		bytes.NewBufferString(`{"permissions":["manage_tags"]}`),
	)
	forbiddenReq.Header.Set("Content-Type", "application/json")
	forbiddenReq.AddCookie(adminCookie)
	forbiddenRec := httptest.NewRecorder()
	engine.ServeHTTP(forbiddenRec, forbiddenReq)
	if forbiddenRec.Code != http.StatusForbidden {
		t.Fatalf("normal admin should still be forbidden from creating admins, got %d", forbiddenRec.Code)
	}
}

func TestSuperAdminCanPersistSystemSettings(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	superAdmin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, superAdmin)

	body := bytes.NewBufferString(`{
		"guest":{"allow_direct_publish":true,"extra_permissions_enabled":true,"allow_guest_edit_title":false,"allow_guest_edit_tags":false,"allow_guest_edit_description":false,"allow_guest_resource_delete":false},
		"upload":{"max_file_size_bytes":1048576,"max_tag_count":8,"allowed_extensions":[".pdf",".md"]},
		"search":{"enable_fuzzy_match":true,"enable_tag_filter":true,"enable_folder_scope":true,"result_window":25}
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/system/settings", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save system settings: expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/system/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	engine.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get system settings: expected 200, got %d, body=%s", getRec.Code, getRec.Body.String())
	}

	var response struct {
		Guest struct {
			AllowDirectPublish bool `json:"allow_direct_publish"`
		} `json:"guest"`
		Upload struct {
			MaxTagCount int `json:"max_tag_count"`
		} `json:"upload"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode system settings response: %v", err)
	}
	if !response.Guest.AllowDirectPublish || response.Upload.MaxTagCount != 8 {
		t.Fatalf("unexpected system settings response: %+v", response)
	}
}

func TestResourceManagementCanUpdateAndDeleteFile(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "resource-admin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionEditResources,
			model.AdminPermissionDeleteResources,
		},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookie := mustCreateSession(t, manager, admin)

	fileID := createTestFile(t, db, nil)
	diskPath := filepath.Join(t.TempDir(), "resource.pdf")
	if err := os.WriteFile(diskPath, []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("write resource file: %v", err)
	}
	if err := db.Model(&model.File{}).Where("id = ?", fileID).Updates(map[string]any{
		"disk_path": diskPath,
	}).Error; err != nil {
		t.Fatalf("update resource disk path: %v", err)
	}

	updateReq := httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources/files/"+fileID,
		bytes.NewBufferString(`{"title":"更新后的标题","description":"新的描述","tags":["Go","Math"]}`),
	)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateRec := httptest.NewRecorder()
	engine.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusNoContent {
		t.Fatalf("update resource: expected 204, got %d, body=%s", updateRec.Code, updateRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/resources/files/"+fileID, nil)
	deleteReq.AddCookie(cookie)
	deleteRec := httptest.NewRecorder()
	engine.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete resource: expected 204, got %d, body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	var file model.File
	if err := db.Where("id = ?", fileID).Take(&file).Error; err != nil {
		t.Fatalf("reload resource file: %v", err)
	}
	if file.Status != model.ResourceStatusDeleted {
		t.Fatalf("expected file status deleted, got %s", file.Status)
	}
}

func TestPublicGuestResourceEditAndDeleteFollowSystemPolicy(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	file := createRepositoryFileForDownload(t, cfg, db, model.ResourceStatusActive, "guest-edit.txt", []byte("hello"))
	file.MimeType = "text/plain"
	if err := db.Save(file).Error; err != nil {
		t.Fatalf("save guest-edit file failed: %v", err)
	}
	engine := New(db, cfg, newRouterSessionManager(db))

	updateReq := httptest.NewRequest(
		http.MethodPut,
		"/api/public/files/"+file.ID,
		bytes.NewBufferString(`{"title":"游客修改后的标题","description":"修改描述","tags":["guest","edit"]}`),
	)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	engine.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusForbidden {
		t.Fatalf("expected guest edit forbidden before policy enabled, got %d, body=%s", updateRec.Code, updateRec.Body.String())
	}

	setGuestResourcePolicy(t, db, true, true, true, true)

	updateReq = httptest.NewRequest(
		http.MethodPut,
		"/api/public/files/"+file.ID,
		bytes.NewBufferString(`{"title":"游客修改后的标题","description":"修改描述","tags":["guest","edit"]}`),
	)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec = httptest.NewRecorder()
	engine.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusNoContent {
		t.Fatalf("expected guest edit success, got %d, body=%s", updateRec.Code, updateRec.Body.String())
	}

	var updated model.File
	if err := db.Where("id = ?", file.ID).Take(&updated).Error; err != nil {
		t.Fatalf("reload updated file failed: %v", err)
	}
	if updated.Title != "游客修改后的标题" || updated.Description != "修改描述" {
		t.Fatalf("unexpected updated file: %+v", updated)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/public/files/"+file.ID, nil)
	deleteRec := httptest.NewRecorder()
	engine.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected guest delete success, got %d, body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	if err := db.Where("id = ?", file.ID).Take(&updated).Error; err != nil {
		t.Fatalf("reload deleted file failed: %v", err)
	}
	if updated.Status != model.ResourceStatusDeleted {
		t.Fatalf("expected guest deleted file status deleted, got %s", updated.Status)
	}
}

func TestOperationLogsVisibleToNormalAdmin(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	superAdmin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "auditor",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
	})
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	superCookie := mustCreateSession(t, manager, superAdmin)
	adminCookie := mustCreateSession(t, manager, admin)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/announcements",
		bytes.NewBufferString(`{"title":"日志公告","content":"写入一条日志","status":"published"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(superCookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create announcement for log setup: expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/operation-logs?page=1&page_size=20", nil)
	listReq.AddCookie(adminCookie)
	listRec := httptest.NewRecorder()
	engine.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list operation logs: expected 200, got %d, body=%s", listRec.Code, listRec.Body.String())
	}

	var response struct {
		Items []struct {
			Action    string `json:"action"`
			AdminName string `json:"admin_name"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode operation logs response: %v", err)
	}
	if response.Total == 0 || len(response.Items) == 0 {
		t.Fatalf("expected at least one operation log, got %+v", response)
	}
	if response.Items[0].Action == "" {
		t.Fatalf("expected action in operation log, got %+v", response.Items[0])
	}
}

func setGuestResourcePolicy(t *testing.T, db *gorm.DB, allowTitle bool, allowTags bool, allowDescription bool, allowDelete bool) {
	t.Helper()

	payload := `{"guest":{"allow_direct_publish":false,"extra_permissions_enabled":true,"allow_guest_edit_title":` +
		boolJSON(allowTitle) + `,"allow_guest_edit_tags":` + boolJSON(allowTags) +
		`,"allow_guest_edit_description":` + boolJSON(allowDescription) +
		`,"allow_guest_resource_delete":` + boolJSON(allowDelete) +
		`},"upload":{"max_file_size_bytes":10485760,"max_tag_count":0,"allowed_extensions":[]},"search":{"enable_fuzzy_match":true,"enable_tag_filter":true,"enable_folder_scope":true,"result_window":50}}`

	var existing model.SystemSetting
	if err := db.Where("key = ?", "system_policy").Take(&existing).Error; err == nil {
		if err := db.Model(&model.SystemSetting{}).Where("key = ?", "system_policy").Updates(map[string]any{"value": payload}).Error; err != nil {
			t.Fatalf("update guest resource policy failed: %v", err)
		}
		return
	}

	if err := db.Create(&model.SystemSetting{
		Key:   "system_policy",
		Value: payload,
	}).Error; err != nil {
		t.Fatalf("create guest resource policy failed: %v", err)
	}
}

func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
