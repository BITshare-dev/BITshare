package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Pragma represents a single SQLite PRAGMA setting.
type Pragma struct {
	Name  string
	Value string
}

// Options holds all parameters needed to open a SQLite database via GORM.
// Defined here so that pkg/database has no dependency on internal/config.
type Options struct {
	Path      string
	LogLevel  string
	EnableWAL bool
	Pragmas   []Pragma
}

// allowedPragmas is a whitelist of safe PRAGMA names to prevent injection.
var allowedPragmas = map[string]bool{
	"foreign_keys":       true,
	"busy_timeout":       true,
	"journal_mode":       true,
	"cache_size":         true,
	"synchronous":        true,
	"temp_store":         true,
	"mmap_size":          true,
	"wal_autocheckpoint": true,
}

func NewSQLite(opts Options) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(opts.Path), 0o755); err != nil {
		return nil, fmt.Errorf("prepare database directory: %w", err)
	}

	dsn, err := buildDSN(opts)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: newLogger(opts.LogLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("fetch sql.DB: %w", err)
	}

	configurePool(sqlDB)

	if err := applyPragmas(db, opts.Pragmas); err != nil {
		return nil, err
	}
	if err := ping(sqlDB); err != nil {
		return nil, err
	}

	return db, nil
}

func buildDSN(opts Options) (string, error) {
	parts := strings.SplitN(opts.Path, "?", 2)
	basePath := parts[0]

	var values url.Values
	if len(parts) == 2 {
		var err error
		values, err = url.ParseQuery(parts[1])
		if err != nil {
			return "", fmt.Errorf("parse sqlite dsn query: %w", err)
		}
	}

	if opts.EnableWAL {
		if values == nil {
			values = make(url.Values)
		}
		values.Set("_journal_mode", "WAL")
	} else if values != nil {
		values.Del("_journal_mode")
	}

	if len(values) == 0 {
		return basePath, nil
	}
	return basePath + "?" + values.Encode(), nil
}

func newLogger(level string) gormlogger.Interface {
	var logLevel gormlogger.LogLevel

	switch strings.ToLower(level) {
	case "silent":
		logLevel = gormlogger.Silent
	case "error":
		logLevel = gormlogger.Error
	case "info":
		logLevel = gormlogger.Info
	default:
		logLevel = gormlogger.Warn
	}

	return gormlogger.Default.LogMode(logLevel)
}

func configurePool(sqlDB *sql.DB) {
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
}

func applyPragmas(db *gorm.DB, pragmas []Pragma) error {
	for _, pragma := range pragmas {
		if !allowedPragmas[pragma.Name] {
			return fmt.Errorf("unsupported sqlite pragma: %q", pragma.Name)
		}
		statement := fmt.Sprintf("PRAGMA %s = %s", pragma.Name, pragma.Value)
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("apply sqlite pragma %q: %w", pragma.Name, err)
		}
	}
	return nil
}

func ping(sqlDB *sql.DB) error {
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}
	return nil
}
