package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"openshare/backend/internal/model"
)

func TestPublicUploadCreatesPendingSubmission(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "默认目录")

	body, contentType := buildUploadRequestBody(t, uploadRequestBody{
		description: "第一章到第四章",
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
	if submission.Name != "notes.pdf" {
		t.Fatalf("expected submission name %q, got %q", "notes.pdf", submission.Name)
	}

	if submission.StagingPath == "" {
		t.Fatal("expected pending submission staging path")
	}
	if _, err := os.Stat(submission.StagingPath); err != nil {
		t.Fatalf("expected staged file to exist, got %v", err)
	}
	if submission.FileID != nil {
		t.Fatalf("expected no formal file link before approval, got %v", *submission.FileID)
	}
	var fileCount int64
	if err := db.Model(&model.File{}).Count(&fileCount).Error; err != nil {
		t.Fatalf("count managed files failed: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected no formal file row before approval, got %d", fileCount)
	}
}

func TestPublicUploadAcceptsAnyExtension(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
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
	db, engine := newPublicUploadTestEnv(t)
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
	if submission.Name != "线性代数讲义.pdf" {
		t.Fatalf("expected name derived from filename, got %q", submission.Name)
	}
}

func TestPublicUploadAssignsFileToFolder(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
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

	var submission model.Submission
	if err := db.Where("name = ?", "probability.pdf").Take(&submission).Error; err != nil {
		t.Fatalf("query submission failed: %v", err)
	}
	if submission.FolderID == nil || *submission.FolderID != folderID {
		t.Fatalf("expected folder_id %q, got %v", folderID, submission.FolderID)
	}
}

func TestPublicUploadAcceptsManifestBatchWithRelativePaths(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "批量目录")

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `[{"relative_path":"课程资料/高数/notes.pdf"},{"relative_path":"课程资料/高数/习题.docx"}]`,
		files: []uploadBatchFile{
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 batch document")},
			{fileName: "习题.docx", fileContent: []byte("PK test")},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var submissions []model.Submission
	if err := db.Order("name ASC").Find(&submissions).Error; err != nil {
		t.Fatalf("query submissions failed: %v", err)
	}
	if len(submissions) != 2 {
		t.Fatalf("expected 2 submissions, got %d", len(submissions))
	}
	if submissions[0].ReceiptCode != submissions[1].ReceiptCode {
		t.Fatalf("expected batch submissions to share receipt code, got %q and %q", submissions[0].ReceiptCode, submissions[1].ReceiptCode)
	}
	if submissions[0].RelativePath == "" || submissions[1].RelativePath == "" {
		t.Fatalf("expected relative paths to be stored, got %+v", submissions)
	}

	expectedPaths := map[string]string{
		"notes.pdf": "课程资料/高数/notes.pdf",
		"习题.docx":   "课程资料/高数/习题.docx",
	}
	for _, submission := range submissions {
		if submission.RelativePath != expectedPaths[submission.Name] {
			t.Fatalf("unexpected relative path for %q: %q", submission.Name, submission.RelativePath)
		}
	}
}
