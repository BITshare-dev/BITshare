package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
)

func migrateSubmissionsSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable("submissions") {
		return nil
	}

	submissionColumns, err := tableColumns(db, "submissions")
	if err != nil {
		return fmt.Errorf("inspect submissions columns: %w", err)
	}
	if len(submissionColumns) == 0 {
		return nil
	}

	if hasTableColumn(submissionColumns, "name") &&
		hasTableColumn(submissionColumns, "description") &&
		hasTableColumn(submissionColumns, "relative_path") &&
		hasTableColumn(submissionColumns, "review_reason") &&
		!hasTableColumn(submissionColumns, "title_snapshot") &&
		!hasTableColumn(submissionColumns, "description_snapshot") &&
		!hasTableColumn(submissionColumns, "relative_path_snapshot") &&
		!hasTableColumn(submissionColumns, "original_name") &&
		!hasTableColumn(submissionColumns, "stored_name") &&
		!hasTableColumn(submissionColumns, "reject_reason") {
		return normalizeSubmissionRelativePaths(db)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DROP TABLE IF EXISTS submissions__new`).Error; err != nil {
			return fmt.Errorf("drop temp submissions table: %w", err)
		}

		if err := tx.Exec(`
			CREATE TABLE submissions__new (
				id TEXT PRIMARY KEY,
				receipt_code TEXT NOT NULL,
				folder_id TEXT,
				file_id TEXT,
				name TEXT NOT NULL DEFAULT '',
				description TEXT NOT NULL DEFAULT '',
				relative_path TEXT NOT NULL DEFAULT '',
				extension TEXT NOT NULL DEFAULT '',
				mime_type TEXT NOT NULL DEFAULT '',
				size INTEGER NOT NULL DEFAULT 0,
				staging_path TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending',
				review_reason TEXT NOT NULL DEFAULT '',
				uploader_ip TEXT NOT NULL DEFAULT '',
				reviewer_id TEXT,
				reviewed_at DATETIME,
				created_at DATETIME,
				updated_at DATETIME
			)
		`).Error; err != nil {
			return fmt.Errorf("create new submissions table: %w", err)
		}

		if err := tx.Exec(`
			INSERT INTO submissions__new (
				id,
				receipt_code,
				folder_id,
				file_id,
				name,
				description,
				relative_path,
				extension,
				mime_type,
				size,
				staging_path,
				status,
					review_reason,
					uploader_ip,
				reviewer_id,
				reviewed_at,
				created_at,
				updated_at
			)
			SELECT
				id,
				COALESCE(receipt_code, ''),
				folder_id,
				file_id,
				COALESCE(
					NULLIF(TRIM(name), ''),
					NULLIF(TRIM(original_name), ''),
					NULLIF(TRIM(stored_name), ''),
					CASE
						WHEN NULLIF(TRIM(title_snapshot), '') IS NOT NULL THEN title_snapshot || COALESCE(extension, '')
						ELSE NULL
					END,
					id
				) AS name,
				COALESCE(description, description_snapshot, ''),
				COALESCE(relative_path, relative_path_snapshot, ''),
				LOWER(COALESCE(extension, '')),
				COALESCE(mime_type, ''),
				COALESCE(size, 0),
				COALESCE(staging_path, ''),
				COALESCE(status, 'pending'),
					COALESCE(review_reason, reject_reason, ''),
					COALESCE(uploader_ip, ''),
				reviewer_id,
				reviewed_at,
				created_at,
				updated_at
			FROM submissions
		`).Error; err != nil {
			return fmt.Errorf("copy submissions into new table: %w", err)
		}

		if err := tx.Exec(`DROP TABLE submissions`).Error; err != nil {
			return fmt.Errorf("drop legacy submissions table: %w", err)
		}
		if err := tx.Exec(`ALTER TABLE submissions__new RENAME TO submissions`).Error; err != nil {
			return fmt.Errorf("rename new submissions table: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return normalizeSubmissionRelativePaths(db)
}

func normalizeSubmissionRelativePaths(db *gorm.DB) error {
	if !db.Migrator().HasTable("submissions") {
		return nil
	}

	type submissionPathRow struct {
		ID           string  `gorm:"column:id"`
		FolderID     *string `gorm:"column:folder_id"`
		RelativePath string  `gorm:"column:relative_path"`
	}

	var rows []submissionPathRow
	if err := db.Model(&model.Submission{}).
		Select("id, folder_id, relative_path").
		Where("COALESCE(TRIM(relative_path), '') <> ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("load submission relative paths: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	ctx := context.Background()
	displayPathByFolder := make(map[string]string)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			rootDisplayPath := ""
			if row.FolderID != nil && strings.TrimSpace(*row.FolderID) != "" {
				folderID := strings.TrimSpace(*row.FolderID)
				var exists bool
				rootDisplayPath, exists = displayPathByFolder[folderID]
				if !exists {
					path, err := repository.BuildFolderDisplayPath(ctx, tx, row.FolderID)
					if err != nil {
						return fmt.Errorf("build submission folder display path: %w", err)
					}
					rootDisplayPath = path
					displayPathByFolder[folderID] = rootDisplayPath
				}
			}

			normalized := repository.NormalizeStoredSubmissionRelativePath(rootDisplayPath, row.RelativePath)
			if normalized == repository.NormalizeRelativePathForStorage(row.RelativePath) {
				continue
			}
			if err := tx.Model(&model.Submission{}).
				Where("id = ?", row.ID).
				UpdateColumn("relative_path", normalized).
				Error; err != nil {
				return fmt.Errorf("normalize submission relative path: %w", err)
			}
		}
		return nil
	})
}
