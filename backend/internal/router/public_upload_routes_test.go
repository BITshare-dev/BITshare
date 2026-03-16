package router

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func TestPublicUploadCreatesPendingSubmission(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "默认目录")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		description: "第一章到第四章",
		tags:        []string{"数学", "考试"},
		folderID:    folderID,
		fileName:    "notes.pdf",
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
		Status      string `json:"status"`
		Title       string `json:"title"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.ReceiptCode == "" {
		t.Fatal("expected generated receipt code")
	}
	if response.Status != string(model.SubmissionStatusPending) {
		t.Fatalf("expected pending status, got %q", response.Status)
	}

	var submission model.Submission
	if err := db.Where("receipt_code = ?", response.ReceiptCode).Take(&submission).Error; err != nil {
		t.Fatalf("query submission failed: %v", err)
	}
	if submission.TitleSnapshot != "notes" {
		t.Fatalf("expected title snapshot 'notes' (from filename), got %q", submission.TitleSnapshot)
	}

	var file model.File
	if err := db.Where("submission_id = ?", submission.ID).Take(&file).Error; err != nil {
		t.Fatalf("query file failed: %v", err)
	}
	if file.Status != model.ResourceStatusOffline {
		t.Fatalf("expected offline file status, got %q", file.Status)
	}
	if _, err := os.Stat(file.DiskPath); err != nil {
		t.Fatalf("expected staged file to exist, got %v", err)
	}
}

func TestPublicUploadReusesExistingReceiptCode(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "默认目录")

	createPendingSubmissionForTest(t, db, "CUSTOM123")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		receiptCode: "CUSTOM123",
		folderID:    folderID,
		fileName:    "os.pdf",
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
	if response.ReceiptCode != "CUSTOM123" {
		t.Fatalf("expected reused receipt code CUSTOM123, got %q", response.ReceiptCode)
	}

	var count int64
	db.Model(&model.Submission{}).Where("receipt_code = ?", "CUSTOM123").Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 submissions sharing receipt code, got %d", count)
	}
}

func TestPublicUploadAcceptsAnyExtension(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "默认目录")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		folderID:    folderID,
		fileName:    "script.sh",
		fileContent: []byte("#!/bin/sh\necho test"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadDerivesGitleFromFilename(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "默认目录")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		folderID:    folderID,
		fileName:    "线性代数讲义.pdf",
		fileContent: []byte("%PDF-1.4 test document"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var submission model.Submission
	if err := db.First(&submission).Error; err != nil {
		t.Fatalf("query submission failed: %v", err)
	}
	if submission.TitleSnapshot != "线性代数讲义" {
		t.Fatalf("expected title derived from filename, got %q", submission.TitleSnapshot)
	}
}

func TestPublicUploadAssignsFileToFolder(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	folderID := createPublicTestFolder(t, db, "课程资料")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		folderID:    folderID,
		fileName:    "probability.pdf",
		fileContent: []byte("%PDF-1.4 test document"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var file model.File
	if err := db.Where("title = ?", "probability").Take(&file).Error; err != nil {
		t.Fatalf("query file failed: %v", err)
	}
	if file.FolderID == nil || *file.FolderID != folderID {
		t.Fatalf("expected folder_id %q, got %v", folderID, file.FolderID)
	}
}

func TestPublicUploadRequiresFolderSelection(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		fileName:    "notes.pdf",
		fileContent: []byte("%PDF-1.4 test document"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadSupportsDirectPublishPolicy(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "直发目录")

	setDirectPublishPolicy(t, db)

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
	if response.Status != string(model.SubmissionStatusApproved) {
		t.Fatalf("expected approved status, got %q", response.Status)
	}

	var submission model.Submission
	if err := db.Where("receipt_code = ?", response.ReceiptCode).Take(&submission).Error; err != nil {
		t.Fatalf("query submission failed: %v", err)
	}
	if submission.Status != model.SubmissionStatusApproved {
		t.Fatalf("expected approved submission, got %q", submission.Status)
	}

	var file model.File
	if err := db.Where("submission_id = ?", submission.ID).Take(&file).Error; err != nil {
		t.Fatalf("query file failed: %v", err)
	}
	if file.Status != model.ResourceStatusActive {
		t.Fatalf("expected active file, got %q", file.Status)
	}
	if _, err := os.Stat(file.DiskPath); err != nil {
		t.Fatalf("expected published file to exist, got %v", err)
	}
}

type uploadRequestBody struct {
	description string
	tags        []string
	receiptCode string
	folderID    string
	fileName    string
	fileContent []byte
}

func buildUploadRequestBody(t *testing.T, input uploadRequestBody) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if input.description != "" {
		if err := writer.WriteField("description", input.description); err != nil {
			t.Fatalf("write description failed: %v", err)
		}
	}
	if input.receiptCode != "" {
		if err := writer.WriteField("receipt_code", input.receiptCode); err != nil {
			t.Fatalf("write receipt code failed: %v", err)
		}
	}
	if input.folderID != "" {
		if err := writer.WriteField("folder_id", input.folderID); err != nil {
			t.Fatalf("write folder_id failed: %v", err)
		}
	}
	for _, tag := range input.tags {
		if err := writer.WriteField("tag", tag); err != nil {
			t.Fatalf("write tag failed: %v", err)
		}
	}

	part, err := writer.CreateFormFile("file", input.fileName)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write(input.fileContent); err != nil {
		t.Fatalf("write file content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	return body, writer.FormDataContentType()
}

func createPendingSubmissionForTest(t *testing.T, db *gorm.DB, receiptCode string) *model.Submission {
	t.Helper()

	submissionID := mustNewID(t)
	submission := &model.Submission{
		ID:            submissionID,
		ReceiptCode:   receiptCode,
		TitleSnapshot: "existing",
		TagsSnapshot:  "[]",
		Status:        model.SubmissionStatusPending,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create pending submission failed: %v", err)
	}

	return submission
}

func setDirectPublishPolicy(t *testing.T, db *gorm.DB) {
	t.Helper()

	payload := `{"guest":{"allow_direct_publish":true,"extra_permissions_enabled":false,"allow_guest_edit_title":false,"allow_guest_edit_tags":false,"allow_guest_edit_description":false,"allow_guest_resource_delete":false},"upload":{"max_file_size_bytes":10485760,"max_tag_count":0,"allowed_extensions":[]},"search":{"enable_fuzzy_match":true,"enable_tag_filter":true,"enable_folder_scope":true,"result_window":50}}`
	if err := db.Create(&model.SystemSetting{
		Key:       "system_policy",
		Value:     payload,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create direct publish system policy failed: %v", err)
	}
}
