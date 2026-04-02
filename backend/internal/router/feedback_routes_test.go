package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func createFeedbackTestFile(t *testing.T, db *gorm.DB) *model.File {
	t.Helper()

	now := time.Now().UTC()
	folderID := mustNewID(t)
	folder := &model.Folder{
		ID:        folderID,
		Name:      "test-folder",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create folder failed: %v", err)
	}

	file := &model.File{
		ID:        mustNewID(t),
		FolderID:  &folderID,
		Name:      "test.pdf",
		Extension: "pdf",
		MimeType:  "application/pdf",
		Size:      1024,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	return file
}

func createFeedbackTestFolder(t *testing.T, db *gorm.DB) *model.Folder {
	t.Helper()

	now := time.Now().UTC()
	folder := &model.Folder{
		ID:        mustNewID(t),
		Name:      "feedback-folder",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	return folder
}

func TestCreateFeedbackForFile(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createFeedbackTestFile(t, db)

	body := bytes.NewBufferString(`{"file_id":"` + file.ID + `","description":"资料内容有误"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/public/feedback", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		FeedbackID  string `json:"feedback_id"`
		ReceiptCode string `json:"receipt_code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.FeedbackID == "" || response.ReceiptCode == "" {
		t.Fatalf("unexpected response payload: %+v", response)
	}

	var feedback model.Feedback
	if err := db.Where("id = ?", response.FeedbackID).Take(&feedback).Error; err != nil {
		t.Fatalf("find feedback failed: %v", err)
	}
	if feedback.TargetType != "file" || feedback.TargetName != file.Name {
		t.Fatalf("unexpected feedback snapshot: type=%q name=%q", feedback.TargetType, feedback.TargetName)
	}
}

func TestCreateFeedbackForFolder(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	folder := createFeedbackTestFolder(t, db)

	body := bytes.NewBufferString(`{"folder_id":"` + folder.ID + `","description":"目录结构需要调整"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/public/feedback", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLookupFeedbackReceiptCode(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createFeedbackTestFile(t, db)
	now := time.Now().UTC()
	feedback := &model.Feedback{
		ID:          mustNewID(t),
		ReceiptCode: "RECEIPT66",
		FileID:      &file.ID,
		TargetName:  file.Name,
		TargetPath:  "test-folder/test.pdf",
		TargetType:  "file",
		Description: "侵权内容",
		ReporterIP:  "127.0.0.1",
		Status:      model.FeedbackStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(feedback).Error; err != nil {
		t.Fatalf("create feedback failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/public/feedback/RECEIPT66", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		ReceiptCode string `json:"receipt_code"`
		Items       []struct {
			TargetName   string `json:"target_name"`
			TargetPath   string `json:"target_path"`
			Status       string `json:"status"`
			ReviewReason string `json:"review_reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.ReceiptCode != "RECEIPT66" || len(response.Items) != 1 {
		t.Fatalf("unexpected response payload: %+v", response)
	}
	if response.Items[0].Status != string(model.FeedbackStatusPending) {
		t.Fatalf("unexpected feedback status %q", response.Items[0].Status)
	}
}

func TestListFeedbackForAdmin(t *testing.T) {
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
	file := createFeedbackTestFile(t, db)
	now := time.Now().UTC()
	feedback := &model.Feedback{
		ID:          mustNewID(t),
		ReceiptCode: "FDBK0001",
		FileID:      &file.ID,
		TargetName:  file.Name,
		TargetPath:  "test-folder/test.pdf",
		TargetType:  "file",
		Description: "需要修正文档",
		ReporterIP:  "127.0.0.1",
		Status:      model.FeedbackStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(feedback).Error; err != nil {
		t.Fatalf("create feedback failed: %v", err)
	}

	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			ReceiptCode string `json:"receipt_code"`
			TargetName  string `json:"target_name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].ReceiptCode != "FDBK0001" {
		t.Fatalf("unexpected response payload: %+v", response)
	}
}

func TestApproveFeedback(t *testing.T) {
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
	file := createFeedbackTestFile(t, db)
	now := time.Now().UTC()
	feedback := &model.Feedback{
		ID:          mustNewID(t),
		ReceiptCode: "FDBK0002",
		FileID:      &file.ID,
		TargetName:  file.Name,
		TargetPath:  "test-folder/test.pdf",
		TargetType:  "file",
		Description: "内容需要更正",
		ReporterIP:  "127.0.0.1",
		Status:      model.FeedbackStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(feedback).Error; err != nil {
		t.Fatalf("create feedback failed: %v", err)
	}

	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"review_reason":"已补充说明"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/feedback/"+feedback.ID+"/approve", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updated model.Feedback
	if err := db.Where("id = ?", feedback.ID).Take(&updated).Error; err != nil {
		t.Fatalf("reload feedback failed: %v", err)
	}
	if updated.Status != model.FeedbackStatusApproved {
		t.Fatalf("expected approved status, got %q", updated.Status)
	}
	if updated.ReviewReason != "已补充说明" {
		t.Fatalf("unexpected review reason %q", updated.ReviewReason)
	}
}

func TestRejectFeedback(t *testing.T) {
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
	folder := createFeedbackTestFolder(t, db)
	now := time.Now().UTC()
	feedback := &model.Feedback{
		ID:          mustNewID(t),
		ReceiptCode: "FDBK0003",
		FolderID:    &folder.ID,
		TargetName:  folder.Name,
		TargetPath:  folder.Name,
		TargetType:  "folder",
		Description: "反馈不成立",
		ReporterIP:  "127.0.0.1",
		Status:      model.FeedbackStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(feedback).Error; err != nil {
		t.Fatalf("create feedback failed: %v", err)
	}

	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"review_reason":"经核实反馈不成立"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/feedback/"+feedback.ID+"/reject", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updated model.Feedback
	if err := db.Where("id = ?", feedback.ID).Take(&updated).Error; err != nil {
		t.Fatalf("reload feedback failed: %v", err)
	}
	if updated.Status != model.FeedbackStatusRejected {
		t.Fatalf("expected rejected status, got %q", updated.Status)
	}
	if updated.ReviewReason != "经核实反馈不成立" {
		t.Fatalf("unexpected review reason %q", updated.ReviewReason)
	}
}

func TestRejectFeedbackRequiresReviewReason(t *testing.T) {
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
	folder := createFeedbackTestFolder(t, db)
	now := time.Now().UTC()
	feedback := &model.Feedback{
		ID:          mustNewID(t),
		ReceiptCode: "FDBK0004",
		FolderID:    &folder.ID,
		TargetName:  folder.Name,
		TargetPath:  folder.Name,
		TargetType:  "folder",
		Description: "反馈待驳回",
		ReporterIP:  "127.0.0.1",
		Status:      model.FeedbackStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(feedback).Error; err != nil {
		t.Fatalf("create feedback failed: %v", err)
	}

	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)
	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"review_reason":"   "}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/feedback/"+feedback.ID+"/reject", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updated model.Feedback
	if err := db.Where("id = ?", feedback.ID).Take(&updated).Error; err != nil {
		t.Fatalf("reload feedback failed: %v", err)
	}
	if updated.Status != model.FeedbackStatusPending {
		t.Fatalf("expected pending status to remain unchanged, got %q", updated.Status)
	}
}
