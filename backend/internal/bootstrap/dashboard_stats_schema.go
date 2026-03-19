package bootstrap

import (
	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func rebuildDashboardStats(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		nowExpr := "CURRENT_TIMESTAMP"

		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.DailyStat{}).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.SystemStat{}).Error; err != nil {
			return err
		}

		if err := tx.Exec(`
			INSERT INTO system_stats (
				key, total_visits, total_files, total_downloads, pending_submissions, pending_reports, created_at, updated_at
			)
			SELECT
				?,
				COALESCE((SELECT COUNT(*) FROM site_visit_events), 0),
				COALESCE((SELECT COUNT(*) FROM files WHERE status = ? AND deleted_at IS NULL), 0),
				COALESCE((SELECT COUNT(*) FROM download_events), 0),
				COALESCE((SELECT COUNT(*) FROM submissions WHERE status = ?), 0),
				COALESCE((SELECT COUNT(*) FROM reports WHERE status = ?), 0),
				`+nowExpr+`,
				`+nowExpr+`
		`, model.GlobalSystemStatsKey, model.ResourceStatusActive, model.SubmissionStatusPending, model.ReportStatusPending).Error; err != nil {
			return err
		}

		if err := tx.Exec(`
			INSERT INTO daily_stats (day, new_files, downloads, visits, created_at, updated_at)
			SELECT
				day,
				COALESCE(SUM(new_files), 0),
				COALESCE(SUM(downloads), 0),
				COALESCE(SUM(visits), 0),
				`+nowExpr+`,
				`+nowExpr+`
			FROM (
				SELECT DATE(created_at) AS day, COUNT(*) AS new_files, 0 AS downloads, 0 AS visits
				FROM files
				WHERE status = ? AND deleted_at IS NULL
				GROUP BY DATE(created_at)
				UNION ALL
				SELECT DATE(created_at) AS day, 0 AS new_files, COUNT(*) AS downloads, 0 AS visits
				FROM download_events
				GROUP BY DATE(created_at)
				UNION ALL
				SELECT DATE(created_at) AS day, 0 AS new_files, 0 AS downloads, COUNT(*) AS visits
				FROM site_visit_events
				GROUP BY DATE(created_at)
			) combined
			GROUP BY day
		`, model.ResourceStatusActive).Error; err != nil {
			return err
		}

		return nil
	})
}
