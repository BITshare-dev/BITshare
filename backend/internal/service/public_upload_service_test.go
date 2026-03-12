package service

import (
	"context"
	"errors"
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
	service := NewPublicUploadService(cfg.Upload, repo, storageService)

	createExistingSubmission(t, db, "CUSTOM123")

	result, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		ReceiptCode:  "CUSTOM123",
		OriginalName: "notes.pdf",
		DeclaredMIME: "application/pdf",
		File:         strings.NewReader("%PDF-1.4 test document"),
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
	service := NewPublicUploadService(cfg.Upload, repo, storageService)
	service.codeGen = func(int) (string, error) {
		return "", errors.New("entropy unavailable")
	}

	_, err := service.CreateSubmission(context.Background(), PublicUploadInput{
		OriginalName: "notes.pdf",
		DeclaredMIME: "application/pdf",
		File:         strings.NewReader("%PDF-1.4 test document"),
	})
	if !errors.Is(err, ErrReceiptCodeGenerate) {
		t.Fatalf("expected receipt generation error, got %v", err)
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
		ID:            mustNewUploadID(t),
		ReceiptCode:   receiptCode,
		TitleSnapshot: "existing",
		TagsSnapshot:  "[]",
		Status:        model.SubmissionStatusPending,
	}
	if err := db.Create(submission).Error; err != nil {
		t.Fatalf("create existing submission failed: %v", err)
	}
}

func mustNewUploadID(t *testing.T) string {
	t.Helper()

	id, err := identity.NewID()
	if err != nil {
		t.Fatalf("generate id failed: %v", err)
	}
	return id
}
