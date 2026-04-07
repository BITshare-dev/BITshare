package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPreservesUploadDefaultsForPartialOverrides(t *testing.T) {
	defaultPath, localPath := writeTestConfigFiles(
		t,
		Default(),
		`{"upload":{"max_upload_total_bytes":123456789}}`,
	)

	cfg, err := Load(defaultPath, localPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Upload.MaxUploadTotalBytes != 123456789 {
		t.Fatalf("MaxUploadTotalBytes = %d, want %d", cfg.Upload.MaxUploadTotalBytes, int64(123456789))
	}
	if cfg.Upload.MaxDescriptionLength != Default().Upload.MaxDescriptionLength {
		t.Fatalf("MaxDescriptionLength = %d, want %d", cfg.Upload.MaxDescriptionLength, Default().Upload.MaxDescriptionLength)
	}
	if cfg.Upload.ReceiptCodeLength != Default().Upload.ReceiptCodeLength {
		t.Fatalf("ReceiptCodeLength = %d, want %d", cfg.Upload.ReceiptCodeLength, Default().Upload.ReceiptCodeLength)
	}
}

func TestLoadSupportsLegacyUploadMaxFileSizeBytes(t *testing.T) {
	defaultPath, localPath := writeTestConfigFiles(
		t,
		Default(),
		`{"upload":{"max_file_size_bytes":987654321}}`,
	)

	cfg, err := Load(defaultPath, localPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Upload.MaxUploadTotalBytes != 987654321 {
		t.Fatalf("MaxUploadTotalBytes = %d, want %d", cfg.Upload.MaxUploadTotalBytes, int64(987654321))
	}
	if cfg.Upload.MaxDescriptionLength != Default().Upload.MaxDescriptionLength {
		t.Fatalf("MaxDescriptionLength = %d, want %d", cfg.Upload.MaxDescriptionLength, Default().Upload.MaxDescriptionLength)
	}
	if cfg.Upload.ReceiptCodeLength != Default().Upload.ReceiptCodeLength {
		t.Fatalf("ReceiptCodeLength = %d, want %d", cfg.Upload.ReceiptCodeLength, Default().Upload.ReceiptCodeLength)
	}
}

func TestLoadRejectsExplicitInvalidUploadOverrideEvenWithLegacyFallback(t *testing.T) {
	defaultPath, localPath := writeTestConfigFiles(
		t,
		Default(),
		`{"upload":{"max_upload_total_bytes":0,"max_file_size_bytes":1024}}`,
	)

	_, err := Load(defaultPath, localPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "upload.max_upload_total_bytes must be greater than 0") {
		t.Fatalf("Load() error = %v, want upload.max_upload_total_bytes validation error", err)
	}
}

func writeTestConfigFiles(t *testing.T, cfg Config, localJSON string) (string, string) {
	t.Helper()

	dir := t.TempDir()
	cfg.Session.Secret = "test-session-secret"

	defaultPath := filepath.Join(dir, "config.default.json")
	localPath := filepath.Join(dir, "config.local.json")

	defaultData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if err := os.WriteFile(defaultPath, defaultData, 0o600); err != nil {
		t.Fatalf("write default config: %v", err)
	}
	if err := os.WriteFile(localPath, []byte(localJSON), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	return defaultPath, localPath
}
