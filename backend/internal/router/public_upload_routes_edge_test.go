package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
)

func TestPublicUploadRejectsWhenTotalSizeExceedsLimit(t *testing.T) {
	cfg := newRouterTestConfig(t)
	cfg.Upload.MaxUploadTotalBytes = 16
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))
	folderID := createPublicTestFolder(t, db, "总量限制目录")

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `[{"relative_path":"notes-a.pdf"},{"relative_path":"notes-b.pdf"}]`,
		files: []uploadBatchFile{
			{fileName: "notes-a.pdf", fileContent: []byte("1234567890")},
			{fileName: "notes-b.pdf", fileContent: []byte("abcdefghij")},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadRejectsDuplicateNamesWithinBatch(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "批量目录")

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `[{"relative_path":"课程资料/高数/notes.pdf"},{"relative_path":"课程资料/高数/notes.pdf"}]`,
		files: []uploadBatchFile{
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 batch document")},
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 another batch document")},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadAllowsNamesThatDifferOnlyByCase(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "批量目录")

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `[{"relative_path":"课程资料/高数/notes.pdf"},{"relative_path":"课程资料/高数/Notes.pdf"}]`,
		files: []uploadBatchFile{
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 lower case")},
			{fileName: "Notes.pdf", fileContent: []byte("%PDF-1.4 upper case")},
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
	if err := db.Order("relative_path ASC").Find(&submissions).Error; err != nil {
		t.Fatalf("query submissions failed: %v", err)
	}
	if len(submissions) != 2 {
		t.Fatalf("expected 2 submissions, got %d", len(submissions))
	}
}

func TestPublicUploadRejectsWhenSameFolderAlreadyExistsInTargetDirectory(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "上传目录")

	var folder model.Folder
	if err := db.Where("id = ?", folderID).Take(&folder).Error; err != nil {
		t.Fatalf("load folder failed: %v", err)
	}
	if folder.SourcePath == nil {
		t.Fatal("expected folder source path")
	}

	existingPath := filepath.Join(*folder.SourcePath, "课程资料")
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("seed existing folder failed: %v", err)
	}

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `[{"relative_path":"课程资料/高数/notes.pdf"}]`,
		files: []uploadBatchFile{
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 nested document")},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadAllowsSameFolderNameInDifferentDirectories(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "上传目录")

	var folder model.Folder
	if err := db.Where("id = ?", folderID).Take(&folder).Error; err != nil {
		t.Fatalf("load folder failed: %v", err)
	}
	if folder.SourcePath == nil {
		t.Fatal("expected folder source path")
	}

	existingPath := filepath.Join(*folder.SourcePath, "其他资料", "半导体物理")
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("seed existing sibling folder failed: %v", err)
	}

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `[{"relative_path":"课程资料/半导体物理/notes.pdf"}]`,
		files: []uploadBatchFile{
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 nested document")},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicSubmissionLookupStillWorksAfterFileDeletion(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)

	submission := createPendingSubmissionForTest(t, db, "DELETED88")
	file := createFileForSubmission(t, db, submission.ID, 12)
	if err := db.Delete(&model.File{}, "id = ?", file.ID).Error; err != nil {
		t.Fatalf("delete file failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/public/submissions/DELETED88", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}
