package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

func TestUnmanageManagedDirectoryRequiresSuperAdminPasswordKeepsSourceDirectoryAndCleansManagedData(t *testing.T) {
	_, _, cookie, engine, db := newImportRouteEnv(t, adminAccess{
		username: "superadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleSuperAdmin),
	})
	importRoot := createImportFixture(t)

	importRecorder := importLocalDirectory(t, engine, cookie, importRoot)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("expected import status 200, got %d body=%s", importRecorder.Code, importRecorder.Body.String())
	}

	var rootFolder model.Folder
	if err := db.Where("source_path = ?", importRoot).Take(&rootFolder).Error; err != nil {
		t.Fatalf("find root folder failed: %v", err)
	}

	var importedFile model.File
	if err := db.Where("name = ?", "root.pdf").Take(&importedFile).Error; err != nil {
		t.Fatalf("find imported file failed: %v", err)
	}

	pendingSubmission := &model.Submission{
		ID:          mustNewID(t),
		ReceiptCode: "SUBMIT001",
		FolderID:    &rootFolder.ID,
		Name:        "pending-review.pdf",
		Status:      model.SubmissionStatusPending,
	}
	if err := db.Create(pendingSubmission).Error; err != nil {
		t.Fatalf("create pending submission failed: %v", err)
	}

	approvedSubmission := &model.Submission{
		ID:          mustNewID(t),
		ReceiptCode: "SUBMIT002",
		FileID:      &importedFile.ID,
		Name:        importedFile.Name,
		Status:      model.SubmissionStatusApproved,
	}
	if err := db.Create(approvedSubmission).Error; err != nil {
		t.Fatalf("create approved submission failed: %v", err)
	}

	pendingFeedback := &model.Feedback{
		ID:          mustNewID(t),
		ReceiptCode: "FDBK0001",
		FileID:      &importedFile.ID,
		TargetName:  importedFile.Name,
		TargetPath:  "/import-root/root.pdf",
		TargetType:  "file",
		Description: "needs cleanup",
		Status:      model.FeedbackStatusPending,
	}
	if err := db.Create(pendingFeedback).Error; err != nil {
		t.Fatalf("create pending feedback failed: %v", err)
	}

	if err := repository.NewPublicDownloadRepository(db).IncrementDownloadCount(t.Context(), importedFile.ID); err != nil {
		t.Fatalf("increment download failed: %v", err)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/admin/imports/local/"+rootFolder.ID, bytes.NewBufferString(`{"password":"s3cret-pass"}`))
	deleteRequest.Header.Set("Content-Type", "application/json")
	deleteRequest.AddCookie(cookie)
	deleteRecorder := httptest.NewRecorder()
	engine.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d body=%s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	if _, err := os.Stat(importRoot); err != nil {
		t.Fatalf("expected import root to remain on disk, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(importRoot, "root.pdf")); err != nil {
		t.Fatalf("expected imported root file to remain on disk, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(importRoot, "nested", "chapter1.txt")); err != nil {
		t.Fatalf("expected imported nested file to remain on disk, got %v", err)
	}

	var folderCount int64
	if err := db.Model(&model.Folder{}).Count(&folderCount).Error; err != nil {
		t.Fatalf("count folders failed: %v", err)
	}
	if folderCount != 0 {
		t.Fatalf("expected all imported folders deleted, got %d", folderCount)
	}

	var fileCount int64
	if err := db.Model(&model.File{}).Count(&fileCount).Error; err != nil {
		t.Fatalf("count files failed: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected all imported files deleted, got %d", fileCount)
	}

	var submissionCount int64
	if err := db.Model(&model.Submission{}).Count(&submissionCount).Error; err != nil {
		t.Fatalf("count submissions failed: %v", err)
	}
	if submissionCount != 0 {
		t.Fatalf("expected related submissions deleted, got %d", submissionCount)
	}

	var feedbackCount int64
	if err := db.Model(&model.Feedback{}).Count(&feedbackCount).Error; err != nil {
		t.Fatalf("count feedbacks failed: %v", err)
	}
	if feedbackCount != 0 {
		t.Fatalf("expected related feedback deleted, got %d", feedbackCount)
	}

	var downloadEventCount int64
	if err := db.Model(&model.DownloadEvent{}).Count(&downloadEventCount).Error; err != nil {
		t.Fatalf("count download events failed: %v", err)
	}
	if downloadEventCount != 1 {
		t.Fatalf("expected historical download events preserved, got %d", downloadEventCount)
	}

	var systemStats model.SystemStat
	if err := db.Where("key = ?", model.GlobalSystemStatsKey).Take(&systemStats).Error; err != nil {
		t.Fatalf("load system stats failed: %v", err)
	}
	if systemStats.TotalFiles != 0 {
		t.Fatalf("expected total files reset, got %d", systemStats.TotalFiles)
	}
	if systemStats.TotalDownloads != 1 {
		t.Fatalf("expected historical total downloads preserved, got %d", systemStats.TotalDownloads)
	}
	if systemStats.PendingSubmissions != 0 {
		t.Fatalf("expected pending submissions reset, got %d", systemStats.PendingSubmissions)
	}
	if systemStats.PendingFeedbacks != 0 {
		t.Fatalf("expected pending feedbacks reset, got %d", systemStats.PendingFeedbacks)
	}

	var unmanageLog model.OperationLog
	if err := db.Where("action = ?", "managed_directory_unmanaged").Take(&unmanageLog).Error; err != nil {
		t.Fatalf("find unmanage operation log failed: %v", err)
	}
	if unmanageLog.Detail != importRoot {
		t.Fatalf("expected unmanage log detail %q, got %q", importRoot, unmanageLog.Detail)
	}
}

func TestRescanManagedDirectoryMirrorsFilesystemAndPreservesHistoricalDownloadStats(t *testing.T) {
	_, _, cookie, engine, db := newImportRouteEnv(t, adminAccess{
		username: "superadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleSuperAdmin),
	})
	importRoot := createImportFixture(t)

	importRecorder := importLocalDirectory(t, engine, cookie, importRoot)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("expected import status 200, got %d body=%s", importRecorder.Code, importRecorder.Body.String())
	}

	var rootFolder model.Folder
	if err := db.Where("source_path = ?", importRoot).Take(&rootFolder).Error; err != nil {
		t.Fatalf("find root folder failed: %v", err)
	}

	renamedSourcePath := filepath.Join(importRoot, "nested", "chapter1.txt")
	var downloadedFile model.File
	if err := db.Where("name = ?", "chapter1.txt").Take(&downloadedFile).Error; err != nil {
		t.Fatalf("find file for download failed: %v", err)
	}
	if err := repository.NewPublicDownloadRepository(db).IncrementDownloadCount(t.Context(), downloadedFile.ID); err != nil {
		t.Fatalf("increment download failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(importRoot, "root.pdf"), []byte("root file with larger contents"), 0o644); err != nil {
		t.Fatalf("rewrite root fixture file failed: %v", err)
	}
	renamedPath := filepath.Join(importRoot, "nested", "chapter-renamed.txt")
	if err := os.Rename(renamedSourcePath, renamedPath); err != nil {
		t.Fatalf("rename imported file failed: %v", err)
	}
	newDir := filepath.Join(importRoot, "nested", "newdir")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("create new directory failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "extra.md"), []byte("extra content"), 0o644); err != nil {
		t.Fatalf("write extra file failed: %v", err)
	}

	rescanRequest := httptest.NewRequest(http.MethodPost, "/api/admin/imports/local/"+rootFolder.ID+"/rescan", nil)
	rescanRequest.AddCookie(cookie)
	rescanRecorder := httptest.NewRecorder()
	engine.ServeHTTP(rescanRecorder, rescanRequest)
	if rescanRecorder.Code != http.StatusOK {
		t.Fatalf("expected rescan status 200, got %d body=%s", rescanRecorder.Code, rescanRecorder.Body.String())
	}

	var rescanResult struct {
		AddedFolders   int `json:"added_folders"`
		AddedFiles     int `json:"added_files"`
		UpdatedFiles   int `json:"updated_files"`
		DeletedFiles   int `json:"deleted_files"`
		DeletedFolders int `json:"deleted_folders"`
	}
	if err := json.Unmarshal(rescanRecorder.Body.Bytes(), &rescanResult); err != nil {
		t.Fatalf("decode rescan response failed: %v", err)
	}
	if rescanResult.AddedFolders != 1 || rescanResult.AddedFiles != 2 || rescanResult.UpdatedFiles != 1 || rescanResult.DeletedFiles != 1 || rescanResult.DeletedFolders != 0 {
		t.Fatalf("unexpected rescan result: %+v", rescanResult)
	}

	var missingCount int64
	if err := db.Model(&model.File{}).Where("name = ?", "chapter1.txt").Count(&missingCount).Error; err != nil {
		t.Fatalf("count deleted file failed: %v", err)
	}
	if missingCount != 0 {
		t.Fatalf("expected renamed source file removed from db, got %d rows", missingCount)
	}

	var renamedFile model.File
	if err := db.Where("name = ?", "chapter-renamed.txt").Take(&renamedFile).Error; err != nil {
		t.Fatalf("find renamed file failed: %v", err)
	}
	if renamedFile.DownloadCount != 0 {
		t.Fatalf("expected renamed file download count reset, got %d", renamedFile.DownloadCount)
	}

	var updatedRootFile model.File
	if err := db.Where("name = ?", "root.pdf").Take(&updatedRootFile).Error; err != nil {
		t.Fatalf("find updated root file failed: %v", err)
	}
	if updatedRootFile.Size <= int64(len("root file")) {
		t.Fatalf("expected root file size updated, got %d", updatedRootFile.Size)
	}

	var extraFile model.File
	if err := db.Where("name = ?", "extra.md").Take(&extraFile).Error; err != nil {
		t.Fatalf("find extra file failed: %v", err)
	}
	if extraFile.FolderID == nil {
		t.Fatalf("expected extra file folder tracked, got nil folder id")
	}
	var extraFolder model.Folder
	if err := db.Where("id = ?", *extraFile.FolderID).Take(&extraFolder).Error; err != nil {
		t.Fatalf("find extra file folder failed: %v", err)
	}
	if extraFolder.SourcePath == nil || *extraFolder.SourcePath != newDir {
		t.Fatalf("expected extra file folder path %q, got %+v", newDir, extraFolder.SourcePath)
	}

	if err := db.Where("id = ?", rootFolder.ID).Take(&rootFolder).Error; err != nil {
		t.Fatalf("reload root folder failed: %v", err)
	}
	if rootFolder.DownloadCount != 0 {
		t.Fatalf("expected current root folder download count reset to managed resources, got %d", rootFolder.DownloadCount)
	}

	var systemStats model.SystemStat
	if err := db.Where("key = ?", model.GlobalSystemStatsKey).Take(&systemStats).Error; err != nil {
		t.Fatalf("find system stats failed: %v", err)
	}
	if systemStats.TotalDownloads != 1 {
		t.Fatalf("expected historical total downloads preserved, got %d", systemStats.TotalDownloads)
	}
}
