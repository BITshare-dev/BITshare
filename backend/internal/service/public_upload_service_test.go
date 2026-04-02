package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"

	"openshare/backend/internal/bootstrap"
	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/database"
	"openshare/backend/pkg/identity"
)

func TestCreateSubmissionReusesExistingReceiptCode(t *testing.T) {
	cfg, db, storageService := newUploadTestDeps(t)
	repo := repository.NewUploadRepository(db)
	service := NewPublicUploadService(cfg.Upload, repo, NewReceiptCodeService(repository.NewReceiptCodeRepository(db), cfg.Upload.ReceiptCodeLength), storageService, nil)
	folderID := createUploadTargetFolder(t, db)

	createExistingSubmission(t, db, "CUSTOM123")

	result, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		ReceiptCode: "CUSTOM123",
		FolderID:    folderID,
		Files: []PublicUploadFileInput{
			{
				Name: "notes.pdf",
				File: strings.NewReader("%PDF-1.4 test document"),
			},
		},
	})
	if err != nil {
		t.Fatalf("expected success when reusing receipt code, got %v", err)
	}
	if result.ReceiptCode != "CUSTOM123" {
		t.Fatalf("expected receipt code CUSTOM123, got %s", result.ReceiptCode)
	}
}

func TestCreateSubmissionReturnsReceiptGenerationError(t *testing.T) {
	cfg, db, storageService := newUploadTestDeps(t)
	repo := repository.NewUploadRepository(db)
	service := NewPublicUploadService(cfg.Upload, repo, NewReceiptCodeService(repository.NewReceiptCodeRepository(db), cfg.Upload.ReceiptCodeLength), storageService, nil)
	folderID := createUploadTargetFolder(t, db)
	service.receiptCodes.codeGen = func(int) (string, error) {
		return "", errors.New("entropy unavailable")
	}

	_, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		FolderID: folderID,
		Files: []PublicUploadFileInput{
			{
				Name: "notes.pdf",
				File: strings.NewReader("%PDF-1.4 test document"),
			},
		},
	})
	if !errors.Is(err, ErrReceiptCodeGenerate) {
		t.Fatalf("expected receipt generation error, got %v", err)
	}
}

func TestCreateSubmissionIgnoresDSStoreFiles(t *testing.T) {
	cfg, db, storageService := newUploadTestDeps(t)
	repo := repository.NewUploadRepository(db)
	service := NewPublicUploadService(cfg.Upload, repo, NewReceiptCodeService(repository.NewReceiptCodeRepository(db), cfg.Upload.ReceiptCodeLength), storageService, nil)
	folderID := createUploadTargetFolder(t, db)

	result, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		FolderID: folderID,
		Files: []PublicUploadFileInput{
			{
				Name: ".DS_Store",
				File: strings.NewReader("ignored"),
			},
			{
				Name: "notes.pdf",
				File: strings.NewReader("%PDF-1.4 test document"),
			},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.ItemCount != 1 {
		t.Fatalf("expected only 1 uploaded item after filtering, got %d", result.ItemCount)
	}

	var count int64
	if err := db.Model(&model.Submission{}).Count(&count).Error; err != nil {
		t.Fatalf("count submissions failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 stored submission, got %d", count)
	}
}

func TestCreateSubmissionDirectPublishesForModerationAdmin(t *testing.T) {
	cfg, db, storageService := newUploadTestDeps(t)
	repo := repository.NewUploadRepository(db)
	service := NewPublicUploadService(cfg.Upload, repo, NewReceiptCodeService(repository.NewReceiptCodeRepository(db), cfg.Upload.ReceiptCodeLength), storageService, nil)
	folderID := createUploadTargetFolder(t, db)
	adminID := createUploadTestAdmin(t, db)

	ctx := WithPublicUploadActor(context.Background(), PublicUploadActor{
		AdminID:          adminID,
		CanDirectPublish: true,
	})
	result, err := service.CreateSubmission(ctx, PublicUploadInput{
		FolderID: folderID,
		Files: []PublicUploadFileInput{
			{
				Name: "notes.pdf",
				File: strings.NewReader("%PDF-1.4 test document"),
			},
		},
	})
	if err != nil {
		t.Fatalf("expected direct publish success, got %v", err)
	}
	if result.Status != model.SubmissionStatusApproved {
		t.Fatalf("expected approved status, got %q", result.Status)
	}

	var submission model.Submission
	if err := db.Where("name = ?", "notes.pdf").Take(&submission).Error; err != nil {
		t.Fatalf("query submission failed: %v", err)
	}
	if submission.Status != model.SubmissionStatusApproved {
		t.Fatalf("expected approved submission, got %q", submission.Status)
	}
	if submission.FileID == nil {
		t.Fatal("expected approved submission to link managed file")
	}

	var file model.File
	if err := db.Where("id = ?", *submission.FileID).Take(&file).Error; err != nil {
		t.Fatalf("query managed file failed: %v", err)
	}
	var folder model.Folder
	if err := db.Where("id = ?", folderID).Take(&folder).Error; err != nil {
		t.Fatalf("query folder failed: %v", err)
	}
	filePath := model.BuildManagedFilePath(folder.SourcePath, file.Name)
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected managed file to exist, got %v", err)
	}
}

func newUploadTestDeps(t *testing.T) (config.Config, *gorm.DB, *storage.Service) {
	t.Helper()

	cfg := config.Default()
	cfg.Session.Secret = "test-secret"
	cfg.Storage.Root = filepath.Join(t.TempDir(), "storage")
	cfg.Database.Path = filepath.Join(t.TempDir(), "openshare-upload.db")

	if err := storage.EnsureLayout(cfg.Storage); err != nil {
		t.Fatalf("ensure storage layout failed: %v", err)
	}

	db, err := database.NewSQLite(database.Options{
		Path:      cfg.Database.Path,
		LogLevel:  "silent",
		EnableWAL: true,
		Pragmas: []database.Pragma{
			{Name: "foreign_keys", Value: "ON"},
			{Name: "busy_timeout", Value: "5000"},
		},
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := bootstrap.EnsureSchema(db); err != nil {
		t.Fatalf("ensure schema failed: %v", err)
	}

	return cfg, db, storage.NewService(cfg.Storage)
}

func createExistingSubmission(t *testing.T, db *gorm.DB, receiptCode string) {
	t.Helper()

	submission := &model.Submission{
		ID:          mustNewUploadID(t),
		ReceiptCode: receiptCode,
		Name:        "existing.pdf",
		Status:      model.SubmissionStatusPending,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create existing submission failed: %v", err)
	}
}

func createUploadTargetFolder(t *testing.T, db *gorm.DB) string {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(sourcePath, 0o755); err != nil {
		t.Fatalf("ensure upload target folder path failed: %v", err)
	}

	folderID := mustNewUploadID(t)
	folder := &model.Folder{
		ID:         folderID,
		Name:       "upload-target",
		SourcePath: &sourcePath,
	}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create upload target folder failed: %v", err)
	}
	return folderID
}

func createUploadTestAdmin(t *testing.T, db *gorm.DB) string {
	t.Helper()

	adminID := mustNewUploadID(t)
	admin := &model.Admin{
		ID:           adminID,
		Username:     "upload-reviewer",
		DisplayName:  "Upload Reviewer",
		PasswordHash: "hashed-password",
		Role:         string(model.AdminRoleAdmin),
		Permissions:  string(model.AdminPermissionSubmissionModeration),
		Status:       model.AdminStatusActive,
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("create upload test admin failed: %v", err)
	}
	return adminID
}

func mustNewUploadID(t *testing.T) string {
	t.Helper()

	id, err := identity.NewID()
	if err != nil {
		t.Fatalf("generate id failed: %v", err)
	}
	return id
}
