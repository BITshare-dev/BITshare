package router

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func TestPublicUploadCreatesPendingSubmission(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		description: "第一章到第四章",
		tags:        []string{"数学", "考试"},
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

	createPendingSubmissionForTest(t, db, "CUSTOM123")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		receiptCode: "CUSTOM123",
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

func TestPublicUploadRejectsInvalidExtension(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		fileName:    "script.sh",
		fileContent: []byte("#!/bin/sh\necho test"),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadDerivesGitleFromFilename(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
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
