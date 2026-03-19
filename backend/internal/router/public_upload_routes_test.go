package router

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestPublicUploadReusesReceiptCodeFromCookie(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
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

func TestPublicUploadDirectPublishesForSubmissionModerationAdmin(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

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

	var file model.File
	if err := db.Where("original_name = ?", "probability.pdf").Take(&file).Error; err != nil {
		t.Fatalf("query file failed: %v", err)
	}
	if file.Status != model.ResourceStatusActive {
		t.Fatalf("expected active file status, got %q", file.Status)
	}
	if _, err := os.Stat(file.DiskPath); err != nil {
		t.Fatalf("expected file to be moved into managed storage, got %v", err)
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

func TestPublicUploadIgnoresCustomReceiptCodeField(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
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

func TestPublicUploadAcceptsManifestBatchWithRelativePaths(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "批量目录")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("folder_id", folderID); err != nil {
		t.Fatalf("write folder_id failed: %v", err)
	}
	if err := writer.WriteField("manifest", `[{"relative_path":"课程资料/高数/notes.pdf"},{"relative_path":"课程资料/高数/习题.docx"}]`); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}

	partOne, err := writer.CreateFormFile("files", "notes.pdf")
	if err != nil {
		t.Fatalf("create first file failed: %v", err)
	}
	if _, err := partOne.Write([]byte("%PDF-1.4 batch document")); err != nil {
		t.Fatalf("write first file failed: %v", err)
	}

	partTwo, err := writer.CreateFormFile("files", "习题.docx")
	if err != nil {
		t.Fatalf("create second file failed: %v", err)
	}
	if _, err := partTwo.Write([]byte("PK test")); err != nil {
		t.Fatalf("write second file failed: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var submissions []model.Submission
	if err := db.Order("title_snapshot ASC").Find(&submissions).Error; err != nil {
		t.Fatalf("query submissions failed: %v", err)
	}
	if len(submissions) != 2 {
		t.Fatalf("expected 2 submissions, got %d", len(submissions))
	}
	if submissions[0].ReceiptCode != submissions[1].ReceiptCode {
		t.Fatalf("expected batch submissions to share receipt code, got %q and %q", submissions[0].ReceiptCode, submissions[1].ReceiptCode)
	}
	if submissions[0].RelativePathSnapshot == "" || submissions[1].RelativePathSnapshot == "" {
		t.Fatalf("expected relative path snapshots to be stored, got %+v", submissions)
	}
	expectedPaths := map[string]string{
		"notes": "批量目录/课程资料/高数/notes.pdf",
		"习题":    "批量目录/课程资料/高数/习题.docx",
	}
	for _, submission := range submissions {
		if submission.RelativePathSnapshot != expectedPaths[submission.TitleSnapshot] {
			t.Fatalf("unexpected relative path snapshot for %q: %q", submission.TitleSnapshot, submission.RelativePathSnapshot)
		}
	}
}

func TestPublicUploadAutoRenamesDuplicateNamesWithinBatch(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "批量目录")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("folder_id", folderID); err != nil {
		t.Fatalf("write folder_id failed: %v", err)
	}
	if err := writer.WriteField("manifest", `[{"relative_path":"课程资料/高数/notes.pdf"},{"relative_path":"课程资料/高数/notes.pdf"}]`); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}

	partOne, err := writer.CreateFormFile("files", "notes.pdf")
	if err != nil {
		t.Fatalf("create first file failed: %v", err)
	}
	if _, err := partOne.Write([]byte("%PDF-1.4 batch document")); err != nil {
		t.Fatalf("write first file failed: %v", err)
	}

	partTwo, err := writer.CreateFormFile("files", "notes.pdf")
	if err != nil {
		t.Fatalf("create second file failed: %v", err)
	}
	if _, err := partTwo.Write([]byte("%PDF-1.4 another batch document")); err != nil {
		t.Fatalf("write second file failed: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var submissions []model.Submission
	if err := db.Order("relative_path_snapshot ASC").Find(&submissions).Error; err != nil {
		t.Fatalf("query submissions failed: %v", err)
	}
	if len(submissions) != 2 {
		t.Fatalf("expected 2 submissions, got %d", len(submissions))
	}

	gotPaths := []string{submissions[0].RelativePathSnapshot, submissions[1].RelativePathSnapshot}
	expectedPaths := []string{
		"批量目录/课程资料/高数/notes.pdf",
		"批量目录/课程资料/高数/notes_1.pdf",
	}
	for i := range expectedPaths {
		if gotPaths[i] != expectedPaths[i] {
			t.Fatalf("unexpected submission path at %d: got %q want %q", i, gotPaths[i], expectedPaths[i])
		}
	}
}

func TestPublicUploadAutoRenamesWhenManagedDirectoryAlreadyHasSameName(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "直发目录")

	setDirectPublishPolicy(t, db)

	var folder model.Folder
	if err := db.Where("id = ?", folderID).Take(&folder).Error; err != nil {
		t.Fatalf("load folder failed: %v", err)
	}
	if folder.SourcePath == nil {
		t.Fatal("expected folder source path")
	}

	existingPath := filepath.Join(*folder.SourcePath, "direct.pdf")
	if err := os.WriteFile(existingPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed existing file failed: %v", err)
	}

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

	var file model.File
	if err := db.Where("original_name = ?", "direct_1.pdf").Take(&file).Error; err != nil {
		t.Fatalf("query renamed file failed: %v", err)
	}
	if file.Title != "direct_1" {
		t.Fatalf("expected renamed title direct_1, got %q", file.Title)
	}
	if filepath.Base(file.DiskPath) != "direct_1.pdf" {
		t.Fatalf("expected renamed disk file direct_1.pdf, got %q", filepath.Base(file.DiskPath))
	}
}

func TestPublicUploadRejectsInvalidManifest(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "批量目录")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("folder_id", folderID); err != nil {
		t.Fatalf("write folder_id failed: %v", err)
	}
	if err := writer.WriteField("manifest", `{invalid`); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
	part, err := writer.CreateFormFile("files", "notes.pdf")
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	if _, err := part.Write([]byte("%PDF-1.4 batch document")); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadRejectsManifestFileMismatch(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "批量目录")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("folder_id", folderID); err != nil {
		t.Fatalf("write folder_id failed: %v", err)
	}
	if err := writer.WriteField("manifest", `[{"relative_path":"课程资料/高数/notes.pdf"},{"relative_path":"课程资料/高数/习题.docx"}]`); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
	part, err := writer.CreateFormFile("files", "notes.pdf")
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	if _, err := part.Write([]byte("%PDF-1.4 batch document")); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicSubmissionLookupStillWorksAfterFileDeletion(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	submission := createPendingSubmissionForTest(t, db, "DELETED88")
	file := createFileForSubmission(t, db, submission.ID, 12)
	now := time.Now().UTC()
	if err := db.Model(&model.File{}).Where("id = ?", file.ID).Updates(map[string]any{
		"status":     model.ResourceStatusDeleted,
		"deleted_at": &now,
	}).Error; err != nil {
		t.Fatalf("delete file failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/public/submissions/DELETED88", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

type uploadRequestBody struct {
	description string
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
		Status:        model.SubmissionStatusPending,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create pending submission failed: %v", err)
	}

	return submission
}

func setDirectPublishPolicy(t *testing.T, db *gorm.DB) {
	t.Helper()

	payload := `{"guest":{"allow_direct_publish":true,"extra_permissions_enabled":false,"allow_guest_edit_title":false,"allow_guest_edit_description":false,"allow_guest_resource_delete":false},"upload":{"max_file_size_bytes":10485760,"allowed_extensions":[]},"search":{"enable_fuzzy_match":true,"enable_folder_scope":true,"result_window":50}}`
	if err := db.Create(&model.SystemSetting{
		Key:       "system_policy",
		Value:     payload,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create direct publish system policy failed: %v", err)
	}
}
