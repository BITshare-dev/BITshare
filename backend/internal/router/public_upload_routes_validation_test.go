package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicUploadRequiresFolderSelection(t *testing.T) {
	_, engine := newPublicUploadTestEnv(t)

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

func TestPublicUploadRejectsInvalidManifest(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "批量目录")

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `{invalid`,
		files: []uploadBatchFile{
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 batch document")},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUploadRejectsManifestFileMismatch(t *testing.T) {
	db, engine := newPublicUploadTestEnv(t)
	folderID := createPublicTestFolder(t, db, "批量目录")

	body, contentType := buildUploadBatchRequestBody(t, uploadBatchRequestBody{
		folderID: folderID,
		manifest: `[{"relative_path":"课程资料/高数/notes.pdf"},{"relative_path":"课程资料/高数/习题.docx"}]`,
		files: []uploadBatchFile{
			{fileName: "notes.pdf", fileContent: []byte("%PDF-1.4 batch document")},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/public/submissions", body)
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}
