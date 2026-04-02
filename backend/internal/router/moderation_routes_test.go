package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
)

func TestListPendingSubmissions(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	createPendingModerationRecord(t, cfg, db, "PENDING01")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/submissions/pending", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Items []struct {
			ReceiptCode string `json:"receipt_code"`
			Status      string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("expected 1 pending submission, got %d", len(response.Items))
	}
	if response.Items[0].ReceiptCode != "PENDING01" {
		t.Fatalf("unexpected receipt code %q", response.Items[0].ReceiptCode)
	}
}

func TestApproveSubmissionMovesFileAndUpdatesStatus(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	submission, stagingPath := createPendingModerationRecord(t, cfg, db, "APPROVE01")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/submissions/"+submission.ID+"/approve", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updatedSubmission model.Submission
	if err := db.Where("id = ?", submission.ID).Take(&updatedSubmission).Error; err != nil {
		t.Fatalf("reload submission failed: %v", err)
	}
	if updatedSubmission.Status != model.SubmissionStatusApproved {
		t.Fatalf("expected approved status, got %q", updatedSubmission.Status)
	}

	var updatedFile model.File
	if updatedSubmission.FileID == nil {
		t.Fatal("expected approved submission to link to a managed file")
	}
	if err := db.Where("id = ?", *updatedSubmission.FileID).Take(&updatedFile).Error; err != nil {
		t.Fatalf("reload file failed: %v", err)
	}
	if updatedFile.Name != "math.pdf" {
		t.Fatalf("expected file name %q, got %q", "math.pdf", updatedFile.Name)
	}
	var targetFolder model.Folder
	if err := db.Where("id = ?", *updatedFile.FolderID).Take(&targetFolder).Error; err != nil {
		t.Fatalf("reload target folder failed: %v", err)
	}
	if _, err := os.Stat(model.BuildManagedFilePath(targetFolder.SourcePath, updatedFile.Name)); err != nil {
		t.Fatalf("expected file to exist in folder directory: %v", err)
	}
	if _, err := os.Stat(stagingPath); !os.IsNotExist(err) {
		t.Fatalf("expected staged file to be moved, stat err=%v", err)
	}

	var logCount int64
	if err := db.Model(&model.OperationLog{}).Where("target_id = ? AND action = ?", submission.ID, "submission_approved").Count(&logCount).Error; err != nil {
		t.Fatalf("count operation logs failed: %v", err)
	}
	if logCount != 1 {
		t.Fatalf("expected 1 approval log, got %d", logCount)
	}
}

func TestRejectSubmissionDeletesStagedFileAndStoresReason(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	submission, stagingPath := createPendingModerationRecord(t, cfg, db, "REJECT01")
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	body := bytes.NewBufferString(`{"review_reason":"文件内容不完整"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/submissions/"+submission.ID+"/reject", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var updatedSubmission model.Submission
	if err := db.Where("id = ?", submission.ID).Take(&updatedSubmission).Error; err != nil {
		t.Fatalf("reload submission failed: %v", err)
	}
	if updatedSubmission.Status != model.SubmissionStatusRejected {
		t.Fatalf("expected rejected status, got %q", updatedSubmission.Status)
	}
	if updatedSubmission.ReviewReason != "文件内容不完整" {
		t.Fatalf("unexpected review reason %q", updatedSubmission.ReviewReason)
	}
	if _, err := os.Stat(stagingPath); !os.IsNotExist(err) {
		t.Fatalf("expected staged file to be deleted, stat err=%v", err)
	}
	if updatedSubmission.StagingPath != "" {
		t.Fatalf("expected rejected submission staging path to be cleared, got %q", updatedSubmission.StagingPath)
	}
	var approvedFileCount int64
	if err := db.Model(&model.Submission{}).Where("file_id IS NOT NULL").Count(&approvedFileCount).Error; err != nil {
		t.Fatalf("count approved files failed: %v", err)
	}
	if approvedFileCount != 0 {
		t.Fatalf("expected no file row for rejected submission, got %d", approvedFileCount)
	}

	var logCount int64
	if err := db.Model(&model.OperationLog{}).Where("target_id = ? AND action = ?", submission.ID, "submission_rejected").Count(&logCount).Error; err != nil {
		t.Fatalf("count operation logs failed: %v", err)
	}
	if logCount != 1 {
		t.Fatalf("expected 1 rejection log, got %d", logCount)
	}
}

func createPendingModerationRecord(t *testing.T, cfg config.Config, db *gorm.DB, receiptCode string) (*model.Submission, string) {
	t.Helper()

	now := time.Date(2026, 3, 12, 12, 0, 0, 0, time.UTC)
	submissionID := mustNewID(t)
	storedName := mustNewID(t) + ".pdf"
	stagingPath := filepath.Join(cfg.Storage.Root, cfg.Storage.Staging, storedName)
	if err := os.WriteFile(stagingPath, []byte("%PDF-1.4 staged file"), 0o644); err != nil {
		t.Fatalf("write staged file failed: %v", err)
	}

	// Create a target folder with a real directory on disk.
	folderID := mustNewID(t)
	folderDir := filepath.Join(t.TempDir(), "test-folder-"+receiptCode)
	if err := os.MkdirAll(folderDir, 0o755); err != nil {
		t.Fatalf("create folder dir failed: %v", err)
	}
	sourcePath := folderDir
	folder := &model.Folder{
		ID:         folderID,
		Name:       "test-folder",
		SourcePath: &sourcePath,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create folder failed: %v", err)
	}

	submission := &model.Submission{
		ID:           submissionID,
		ReceiptCode:  receiptCode,
		FolderID:     &folderID,
		Name:         "math.pdf",
		Description:  "待审核",
		RelativePath: "math.pdf",
		Extension:    ".pdf",
		MimeType:     "application/pdf",
		Size:         1024,
		StagingPath:  stagingPath,
		Status:       model.SubmissionStatusPending,
		UploaderIP:   "127.0.0.1",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create submission failed: %v", err)
	}

	return submission, stagingPath
}
