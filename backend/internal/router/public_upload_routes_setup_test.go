package router

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/session"
)

type uploadRequestBody struct {
	description string
	receiptCode string
	folderID    string
	fileName    string
	fileContent []byte
}

type uploadBatchRequestBody struct {
	folderID string
	manifest string
	files    []uploadBatchFile
}

type uploadBatchFile struct {
	fieldName   string
	fileName    string
	fileContent []byte
}

func newPublicUploadTestEnv(t *testing.T) (*gorm.DB, http.Handler) {
	t.Helper()

	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	return db, New(db, cfg, newRouterSessionManager(db))
}

func newPublicUploadSessionEnv(t *testing.T) (*gorm.DB, *session.Manager, http.Handler) {
	t.Helper()

	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	return db, manager, New(db, cfg, manager)
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

func buildUploadBatchRequestBody(t *testing.T, input uploadBatchRequestBody) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if input.folderID != "" {
		if err := writer.WriteField("folder_id", input.folderID); err != nil {
			t.Fatalf("write folder_id failed: %v", err)
		}
	}
	if input.manifest != "" {
		if err := writer.WriteField("manifest", input.manifest); err != nil {
			t.Fatalf("write manifest failed: %v", err)
		}
	}

	for _, file := range input.files {
		fieldName := file.fieldName
		if fieldName == "" {
			fieldName = "files"
		}
		part, err := writer.CreateFormFile(fieldName, file.fileName)
		if err != nil {
			t.Fatalf("create file %q failed: %v", file.fileName, err)
		}
		if _, err := part.Write(file.fileContent); err != nil {
			t.Fatalf("write file %q failed: %v", file.fileName, err)
		}
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
		ID:          submissionID,
		ReceiptCode: receiptCode,
		Name:        "existing.pdf",
		Status:      model.SubmissionStatusPending,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create pending submission failed: %v", err)
	}

	return submission
}

func setLegacyDirectPublishPolicy(t *testing.T, db *gorm.DB) {
	t.Helper()

	payload := `{"guest":{"allow_direct_publish":true,"extra_permissions_enabled":false,"allow_guest_edit_title":false,"allow_guest_edit_description":false,"allow_guest_resource_delete":false},"upload":{"max_file_size_bytes":10485760,"allowed_extensions":[]},"search":{"enable_fuzzy_match":true,"enable_folder_scope":true,"result_window":50}}`
	if err := db.Create(&model.SystemSetting{
		Key:   "system_policy",
		Value: payload,
	}).Error; err != nil {
		t.Fatalf("create direct publish system policy failed: %v", err)
	}
}
