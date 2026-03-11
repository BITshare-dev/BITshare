package service

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"openshare/backend/internal/bootstrap"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/database"
)

func TestEnsureDefaultSuperAdminIsIdempotent(t *testing.T) {
	db := newTestSQLite(t)

	adminService := NewAdminBootstrapService(db, repository.NewAdminRepository(db))

	if err := adminService.EnsureDefaultSuperAdmin(); err != nil {
		t.Fatalf("first bootstrap failed: %v", err)
	}
	if err := adminService.EnsureDefaultSuperAdmin(); err != nil {
		t.Fatalf("second bootstrap failed: %v", err)
	}

	var admins []model.Admin
	if err := db.Find(&admins).Error; err != nil {
		t.Fatalf("query admins failed: %v", err)
	}

	if len(admins) != 1 {
		t.Fatalf("expected 1 admin after repeated bootstrap, got %d", len(admins))
	}

	admin := admins[0]
	if admin.Username != defaultSuperAdminUsername {
		t.Fatalf("expected username %q, got %q", defaultSuperAdminUsername, admin.Username)
	}
	if admin.Role != string(model.AdminRoleSuperAdmin) {
		t.Fatalf("expected role %q, got %q", model.AdminRoleSuperAdmin, admin.Role)
	}
	if admin.Status != model.AdminStatusActive {
		t.Fatalf("expected status %q, got %q", model.AdminStatusActive, admin.Status)
	}
	if admin.PasswordHash == "" {
		t.Fatal("expected non-empty password hash")
	}
}

func newTestSQLite(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "openshare-test.db")
	db, err := database.NewSQLite(database.Options{
		Path:      dbPath,
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

	return db
}
