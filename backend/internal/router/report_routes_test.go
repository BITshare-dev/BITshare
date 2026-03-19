package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func createActiveFile(t *testing.T, db *gorm.DB) *model.File {
	t.Helper()
	now := time.Now().UTC()

	folderID := mustNewID(t)
	folder := &model.Folder{
		ID:        folderID,
		Name:      "test-folder",
		Status:    model.ResourceStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}

	file := &model.File{
		ID:           mustNewID(t),
		FolderID:     &folderID,
		Title:        "test-file",
		OriginalName: "test.pdf",
		StoredName:   "stored.pdf",
		Extension:    ".pdf",
		MimeType:     "application/pdf",
		Size:         1024,
		DiskPath:     "/tmp/test.pdf",
		Status:       model.ResourceStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}
	return file
}

func createActiveFolder(t *testing.T, db *gorm.DB) *model.Folder {
	t.Helper()
	now := time.Now().UTC()
	folder := &model.Folder{
		ID:        mustNewID(t),
		Name:      "reportable-folder",
		Status:    model.ResourceStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}
	return folder
}

// ---------------------------------------------------------------------------
// Public: create report
// ---------------------------------------------------------------------------

func TestCreateReportForFile(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)

	body := bytes.NewBufferString(`{"file_id":"` + file.ID + `","reason":"copyright","description":"盗版内容"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/reports", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ReportID    string `json:"report_id"`
		ReceiptCode string `json:"receipt_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ReportID == "" {
		t.Fatal("report_id should not be empty")
	}
	if resp.ReceiptCode == "" {
		t.Fatal("receipt_code should not be empty")
	}

	// Verify DB record
	var report model.Report
	if err := db.Where("id = ?", resp.ReportID).Take(&report).Error; err != nil {
		t.Fatalf("find report: %v", err)
	}
	if report.Reason != "copyright" {
		t.Fatalf("expected reason 'copyright', got %q", report.Reason)
	}
	if report.ReceiptCode != resp.ReceiptCode {
		t.Fatalf("expected stored receipt code %q, got %q", resp.ReceiptCode, report.ReceiptCode)
	}
	if report.TargetName != file.OriginalName || report.TargetType != "file" {
		t.Fatalf("unexpected target snapshot: %q %q", report.TargetName, report.TargetType)
	}
	if report.FileID == nil || *report.FileID != file.ID {
		t.Fatal("report file_id mismatch")
	}
}

func TestLookupReportReceiptCode(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)
	var folder model.Folder
	if err := db.Where("id = ?", *file.FolderID).Take(&folder).Error; err != nil {
		t.Fatalf("load file folder: %v", err)
	}
	now := time.Now().UTC()
	reportID := mustNewID(t)
	report := &model.Report{
		ID:          reportID,
		ReceiptCode: "RECEIPT66",
		FileID:      &file.ID,
		TargetName:  file.OriginalName,
		TargetPath:  folder.Name + "/" + file.OriginalName,
		TargetType:  "file",
		Reason:      "copyright",
		Description: "侵权内容",
		Status:      model.ReportStatusPending,
		ReporterIP:  "127.0.0.1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(report).Error; err != nil {
		t.Fatalf("create report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/public/reports/RECEIPT66", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ReceiptCode string `json:"receipt_code"`
		Items       []struct {
			TargetName string `json:"target_name"`
			TargetPath string `json:"target_path"`
			Status     string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode lookup response: %v", err)
	}
	if resp.ReceiptCode != "RECEIPT66" {
		t.Fatalf("unexpected receipt code %q", resp.ReceiptCode)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 report item, got %d", len(resp.Items))
	}
	if resp.Items[0].TargetName != file.OriginalName {
		t.Fatalf("unexpected target name %q", resp.Items[0].TargetName)
	}
	if resp.Items[0].TargetPath != folder.Name+"/"+file.OriginalName {
		t.Fatalf("unexpected target path %q", resp.Items[0].TargetPath)
	}
	if resp.Items[0].Status != string(model.ReportStatusPending) {
		t.Fatalf("unexpected status %q", resp.Items[0].Status)
	}
}

func TestCreateReportReusesReceiptCodeFromCookie(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)

	body := bytes.NewBufferString(`{"file_id":"` + file.ID + `","reason":"copyright","description":"存在侵权风险"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/reports", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "openshare_receipt_code", Value: "SESSION88"})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ReceiptCode string `json:"receipt_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ReceiptCode != "SESSION88" {
		t.Fatalf("expected cookie receipt code to be reused, got %q", resp.ReceiptCode)
	}
}

func TestCreateReportForFolder(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	folder := createActiveFolder(t, db)

	body := bytes.NewBufferString(`{"folder_id":"` + folder.ID + `","reason":"irrelevant","description":"目录内容与主题无关"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/reports", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateReportRejectsMissingReason(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)

	body := bytes.NewBufferString(`{"file_id":"` + file.ID + `","description":"缺少原因字段"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/reports", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateReportRejectsInvalidReason(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)

	body := bytes.NewBufferString(`{"file_id":"` + file.ID + `","reason":"spam","description":"原因码非法"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/reports", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateReportRejectsBothTargets(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	body := bytes.NewBufferString(`{"file_id":"abc","folder_id":"def","reason":"copyright","description":"同时传了两个目标"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/reports", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateReportRejectsNonexistentTarget(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	body := bytes.NewBufferString(`{"file_id":"nonexistent","reason":"copyright","description":"目标不存在"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/public/reports", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Admin: list / approve / reject
// ---------------------------------------------------------------------------

func TestAdminListPendingReports(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)

	// Create a pending report directly
	reportID := mustNewID(t)
	now := time.Now().UTC()
	db.Create(&model.Report{
		ID:         reportID,
		FileID:     &file.ID,
		Reason:     "copyright",
		Status:     model.ReportStatusPending,
		ReporterIP: "127.0.0.1",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "report-reviewer",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionReviewReports},
	})
	cookieValue, _, _ := manager.Create(t.Context(), admin)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reports/pending", nil)
	req.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Items []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
}

func TestAdminApproveReportMarksHandledWithoutOffliningResource(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)

	reportID := mustNewID(t)
	now := time.Now().UTC()
	db.Create(&model.Report{
		ID:         reportID,
		FileID:     &file.ID,
		Reason:     "copyright",
		Status:     model.ReportStatusPending,
		ReporterIP: "127.0.0.1",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "report-approver",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionReviewReports},
	})
	cookieValue, _, _ := manager.Create(t.Context(), admin)

	body := bytes.NewBufferString(`{"review_reason":"侵权内容已确认"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/reports/"+reportID+"/approve", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// Verify report status
	var report model.Report
	if err := db.Where("id = ?", reportID).Take(&report).Error; err != nil {
		t.Fatalf("reload report: %v", err)
	}
	if report.Status != model.ReportStatusApproved {
		t.Fatalf("expected approved, got %q", report.Status)
	}
	if report.ReviewReason != "侵权内容已确认" {
		t.Fatalf("unexpected stored review reason %q", report.ReviewReason)
	}

	// Verify file is still active
	var updatedFile model.File
	if err := db.Where("id = ?", file.ID).Take(&updatedFile).Error; err != nil {
		t.Fatalf("reload file: %v", err)
	}
	if updatedFile.Status != model.ResourceStatusActive {
		t.Fatalf("expected file still active, got %q", updatedFile.Status)
	}

	// Verify operation log
	var logCount int64
	db.Model(&model.OperationLog{}).Where("target_id = ? AND action = ?", file.ID, "report_approved").Count(&logCount)
	if logCount != 1 {
		t.Fatalf("expected 1 operation log, got %d", logCount)
	}
}

func TestAdminRejectReportKeepsResourceVisible(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	file := createActiveFile(t, db)

	reportID := mustNewID(t)
	now := time.Now().UTC()
	db.Create(&model.Report{
		ID:         reportID,
		FileID:     &file.ID,
		Reason:     "content_error",
		Status:     model.ReportStatusPending,
		ReporterIP: "127.0.0.1",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "report-rejector",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionReviewReports},
	})
	cookieValue, _, _ := manager.Create(t.Context(), admin)

	body := bytes.NewBufferString(`{"review_reason":"经核实内容无误"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/reports/"+reportID+"/reject", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// Verify report status
	var report model.Report
	if err := db.Where("id = ?", reportID).Take(&report).Error; err != nil {
		t.Fatalf("reload report: %v", err)
	}
	if report.Status != model.ReportStatusRejected {
		t.Fatalf("expected rejected, got %q", report.Status)
	}
	if report.ReviewReason != "经核实内容无误" {
		t.Fatalf("unexpected stored review reason %q", report.ReviewReason)
	}

	// Verify file is still active
	var updatedFile model.File
	if err := db.Where("id = ?", file.ID).Take(&updatedFile).Error; err != nil {
		t.Fatalf("reload file: %v", err)
	}
	if updatedFile.Status != model.ResourceStatusActive {
		t.Fatalf("expected file still active, got %q", updatedFile.Status)
	}
}

func TestAdminReportRequiresPermission(t *testing.T) {
	cfg := newRouterTestConfig(t)
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, cfg, manager)

	// Admin without review_reports permission
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "no-report-perm",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{model.AdminPermissionReviewSubmissions},
	})
	cookieValue, _, _ := manager.Create(t.Context(), admin)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reports/pending", nil)
	req.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
