package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"openshare/backend/internal/model"
)

func TestPublicUploadReusesReceiptCodeFromCookie(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "默认目录")

	createPendingSubmissionForTest(t, db, "CUSTOM123")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		folderID:    folderID,
		fileName:    "os.pdf",
		fileContent: []byte("%PDF-1.4 test document"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	request.AddCookie(&http.Cookie{Name: "openshare_receipt_code", Value: "CUSTOM123"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		ReceiptCode string `json:"receipt_code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.ReceiptCode != "CUSTOM123" {
		t.Fatalf("expected reused receipt code CUSTOM123, got %q", response.ReceiptCode)
	}

	var count int64
	db.Model(&model.Submission{}).Where("receipt_code = ?", "CUSTOM123").Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 submissions sharing receipt code, got %d", count)
	}
}

func TestPublicUploadDirectPublishesForSubmissionModerationAdmin(t *testing.T) {
	db, manager, engine := newPublicUploadSessionEnv(t)

	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "reviewer",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionSubmissionModeration},
	})
	cookieValue, _, err := manager.Create(context.Background(), admin)
	if err != nil {
		t.Fatalf("create admin session failed: %v", err)
	}

	folderID := createPublicTestFolder(t, db, "课程资料")
	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		folderID:    folderID,
		fileName:    "probability.pdf",
		fileContent: []byte("%PDF-1.4 test document"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	request.AddCookie(&http.Cookie{Name: "openshare_session", Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.Status != string(model.SubmissionStatusApproved) {
		t.Fatalf("expected approved status, got %q", response.Status)
	}

	var submission model.Submission
	if err := db.Where("name = ?", "probability.pdf").Take(&submission).Error; err != nil {
		t.Fatalf("query submission failed: %v", err)
	}
	if submission.Status != model.SubmissionStatusApproved {
		t.Fatalf("expected approved submission, got %q", submission.Status)
	}
	if submission.FileID == nil {
		t.Fatal("expected direct published submission to link managed file")
	}
	if submission.StagingPath != "" {
		t.Fatalf("expected approved submission to clear staging path, got %q", submission.StagingPath)
	}
	if submission.ReviewerID == nil || *submission.ReviewerID != admin.ID {
		t.Fatalf("expected reviewer_id %q, got %v", admin.ID, submission.ReviewerID)
	}

	var managedFile model.File
	if err := db.Where("id = ?", *submission.FileID).Take(&managedFile).Error; err != nil {
		t.Fatalf("query managed file failed: %v", err)
	}
	var folder model.Folder
	if err := db.Where("id = ?", folderID).Take(&folder).Error; err != nil {
		t.Fatalf("query root folder failed: %v", err)
	}
	filePath := model.BuildManagedFilePath(folder.SourcePath, managedFile.Name)
	if filePath == "" {
		t.Fatal("expected managed file path")
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected managed file to exist on disk, got %v", err)
	}
}

func TestPublicUploadIgnoresLegacyDirectPublishPolicy(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "直发目录")

	setLegacyDirectPublishPolicy(t, db)

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		folderID:    folderID,
		fileName:    "direct.pdf",
		fileContent: []byte("%PDF-1.4 direct document"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		ReceiptCode string `json:"receipt_code"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.Status != string(model.SubmissionStatusPending) {
		t.Fatalf("expected pending status, got %q", response.Status)
	}

	var submission model.Submission
	if err := db.Where("receipt_code = ?", response.ReceiptCode).Take(&submission).Error; err != nil {
		t.Fatalf("query submission failed: %v", err)
	}
	if submission.Status != model.SubmissionStatusPending {
		t.Fatalf("expected pending submission, got %q", submission.Status)
	}
	if submission.FileID != nil {
		t.Fatalf("expected legacy direct publish policy to be ignored, got file_id=%q", *submission.FileID)
	}
}

func TestPublicUploadIgnoresCustomReceiptCodeField(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "默认目录")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		receiptCode: "CUSTOM123",
		folderID:    folderID,
		fileName:    "ignored.pdf",
		fileContent: []byte("%PDF-1.4 test document"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		ReceiptCode string `json:"receipt_code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.ReceiptCode == "CUSTOM123" {
		t.Fatal("expected custom receipt field to be ignored")
	}
}
