package bootstrap

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type pragmaColumnRow struct {
	Name string `gorm:"column:name"`
}

func migrateManagedFilesSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable("files") {
		if err := ensureSubmissionFileIDColumn(db); err != nil {
			return err
		}
		return nil
	}

	fileColumns, err := tableColumns(db, "files")
	if err != nil {
		return fmt.Errorf("inspect files columns: %w", err)
	}
	if len(fileColumns) == 0 {
		return nil
	}

	if err := ensureSubmissionFileIDColumn(db); err != nil {
		return err
	}

	hasLegacySubmissionID := hasTableColumn(fileColumns, "submission_id")
	if hasLegacySubmissionID {
		if err := db.Exec(`
			UPDATE submissions
			SET file_id = (
				SELECT files.id
				FROM files
				WHERE files.submission_id = submissions.id
				LIMIT 1
			)
			WHERE COALESCE(TRIM(file_id), '') = ''
		`).Error; err != nil {
			return fmt.Errorf("backfill submissions.file_id: %w", err)
		}
	}

	if hasTableColumn(fileColumns, "name") &&
		!hasTableColumn(fileColumns, "original_name") &&
		!hasTableColumn(fileColumns, "disk_path") &&
		!hasTableColumn(fileColumns, "submission_id") &&
		!hasTableColumn(fileColumns, "source_path") &&
		!hasTableColumn(fileColumns, "stored_name") &&
		!hasTableColumn(fileColumns, "uploader_ip") &&
		!hasTableColumn(fileColumns, "title") {
		return nil
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DROP TABLE IF EXISTS files__new`).Error; err != nil {
			return fmt.Errorf("drop temp files table: %w", err)
		}

		if err := tx.Exec(`
			CREATE TABLE files__new (
				id TEXT PRIMARY KEY,
				folder_id TEXT,
				name TEXT NOT NULL DEFAULT '',
				description TEXT NOT NULL DEFAULT '',
				extension TEXT NOT NULL DEFAULT '',
				mime_type TEXT NOT NULL DEFAULT '',
				size INTEGER NOT NULL DEFAULT 0,
				download_count INTEGER NOT NULL DEFAULT 0,
				created_at DATETIME,
				updated_at DATETIME
			)
		`).Error; err != nil {
			return fmt.Errorf("create new files table: %w", err)
		}

		if err := tx.Exec(`
			INSERT INTO files__new (
				id,
				folder_id,
				name,
				description,
				extension,
				mime_type,
				size,
				download_count,
				created_at,
				updated_at
			)
			SELECT
				id,
				folder_id,
				COALESCE(
					NULLIF(TRIM(name), ''),
					NULLIF(TRIM(original_name), ''),
					NULLIF(TRIM(stored_name), ''),
					CASE
						WHEN NULLIF(TRIM(title), '') IS NOT NULL THEN title || COALESCE(extension, '')
						ELSE NULL
					END,
					id
				) AS name,
				COALESCE(description, ''),
				LOWER(COALESCE(extension, '')),
				COALESCE(mime_type, ''),
				COALESCE(size, 0),
				COALESCE(download_count, 0),
				created_at,
				updated_at
			FROM files
		`).Error; err != nil {
			return fmt.Errorf("copy files into new table: %w", err)
		}

		if err := tx.Exec(`DROP TABLE files`).Error; err != nil {
			return fmt.Errorf("drop legacy files table: %w", err)
		}
		if err := tx.Exec(`ALTER TABLE files__new RENAME TO files`).Error; err != nil {
			return fmt.Errorf("rename new files table: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func ensureSubmissionFileIDColumn(db *gorm.DB) error {
	if !db.Migrator().HasTable("submissions") || db.Migrator().HasColumn("submissions", "file_id") {
		return nil
	}
	if err := db.Exec(`ALTER TABLE submissions ADD COLUMN file_id TEXT`).Error; err != nil {
		return fmt.Errorf("add submissions.file_id column: %w", err)
	}
	return nil
}

func tableColumns(db *gorm.DB, table string) ([]string, error) {
	var rows []pragmaColumnRow
	if err := db.Raw("PRAGMA table_info(" + table + ")").Scan(&rows).Error; err != nil {
		return nil, err
	}

	columns := make([]string, 0, len(rows))
	for _, row := range rows {
		columns = append(columns, strings.TrimSpace(row.Name))
	}
	return columns, nil
}

func hasTableColumn(columns []string, target string) bool {
	target = strings.TrimSpace(strings.ToLower(target))
	for _, column := range columns {
		if strings.ToLower(strings.TrimSpace(column)) == target {
			return true
		}
	}
	return false
}
