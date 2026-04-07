package bootstrap

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

func rebuildRecentFileDailyDownloads(db *gorm.DB) error {
	if !db.Migrator().HasTable("file_daily_downloads") || !db.Migrator().HasTable("download_events") {
		return nil
	}

	sinceDay := time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM file_daily_downloads WHERE day >= ?`, sinceDay).Error; err != nil {
			return fmt.Errorf("clear recent file daily downloads: %w", err)
		}

		if err := tx.Exec(`
			INSERT INTO file_daily_downloads (file_id, day, downloads, created_at, updated_at)
			SELECT
				file_id,
				DATE(created_at) AS day,
				COUNT(*) AS downloads,
				CURRENT_TIMESTAMP,
				CURRENT_TIMESTAMP
			FROM download_events
			WHERE DATE(created_at) >= ?
			GROUP BY file_id, DATE(created_at)
		`, sinceDay).Error; err != nil {
			return fmt.Errorf("rebuild recent file daily downloads from events: %w", err)
		}

		return nil
	})
}
