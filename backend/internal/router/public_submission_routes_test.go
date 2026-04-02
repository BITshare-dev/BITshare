package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func TestPublicSubmissionLookupByReceiptCode(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	submission := createPendingSubmissionForTest(t, db, "RECEIPT88")
	request := httptest.NewRequest(http.MethodGet, "/api/public/submissions/RECEIPT88", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		ReceiptCode string `json:"receipt_code"`
		Items       []struct {
			Name         string `json:"name"`
			Status       string `json:"status"`
			ReviewReason string `json:"review_reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if response.ReceiptCode != "RECEIPT88" {
		t.Fatalf("unexpected receipt code %q", response.ReceiptCode)
	}
	if len(response.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(response.Items))
	}
	if response.Items[0].Name != submission.Name {
		t.Fatalf("unexpected name %q", response.Items[0].Name)
	}
	if response.Items[0].Status != string(model.SubmissionStatusPending) {
		t.Fatalf("unexpected status %q", response.Items[0].Status)
	}
}

func TestPublicSubmissionLookupReturns404WhenMissing(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	request := httptest.NewRequest(http.MethodGet, "/api/public/submissions/UNKNOWN88", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicSubmissionLookupRejectsInvalidReceiptCode(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	request := httptest.NewRequest(http.MethodGet, "/api/public/submissions/invalid!*", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicSubmissionLookupReturnsMultipleItems(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	engine := New(db, cfg, newRouterSessionManager(db))

	createPendingSubmissionForTest(t, db, "SHARED99")
	createPendingSubmissionForTest(t, db, "SHARED99")

	request := httptest.NewRequest(http.MethodGet, "/api/public/submissions/SHARED99", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		ReceiptCode string `json:"receipt_code"`
		Items       []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if response.ReceiptCode != "SHARED99" {
		t.Fatalf("unexpected receipt code %q", response.ReceiptCode)
	}
	if len(response.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(response.Items))
	}
}

func createFileForSubmission(t *testing.T, db *gorm.DB, submissionID string, downloadCount int64) *model.File {
	t.Helper()

	file := &model.File{
		ID:            mustNewID(t),
		Name:          "existing.pdf",
		Extension:     "pdf",
		MimeType:      "application/pdf",
		Size:          1024,
		DownloadCount: downloadCount,
		CreatedAt:     time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create file for submission failed: %v", err)
	}
	if err := db.Model(&model.Submission{}).Where("id = ?", submissionID).Update("file_id", file.ID).Error; err != nil {
		t.Fatalf("link file to submission failed: %v", err)
	}
	return file
}
