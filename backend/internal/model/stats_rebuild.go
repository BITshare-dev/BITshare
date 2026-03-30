package model

import "gorm.io/gorm"

func RebuildFolderStatsTx(tx *gorm.DB) error {
	if tx == nil {
		return nil
	}

	if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Model(&Folder{}).Updates(map[string]any{
		"file_count":     0,
		"total_size":     0,
		"download_count": 0,
	}).Error; err != nil {
		return err
	}

	query := `
		WITH RECURSIVE folder_tree(root_id, id) AS (
			SELECT id AS root_id, id
			FROM folders
			WHERE status = ?
			UNION ALL
			SELECT folder_tree.root_id, folders.id
			FROM folders
			JOIN folder_tree ON folders.parent_id = folder_tree.id
			WHERE folders.status = ?
		),
		aggregated AS (
			SELECT
				folder_tree.root_id AS folder_id,
				COUNT(files.id) AS file_count,
				COALESCE(SUM(files.size), 0) AS total_size,
				COALESCE(SUM(files.download_count), 0) AS download_count
			FROM folder_tree
			LEFT JOIN files
				ON files.folder_id = folder_tree.id
				AND files.status = ?
				AND files.deleted_at IS NULL
			GROUP BY folder_tree.root_id
		)
		UPDATE folders
		SET
			file_count = COALESCE((SELECT aggregated.file_count FROM aggregated WHERE aggregated.folder_id = folders.id), 0),
			total_size = COALESCE((SELECT aggregated.total_size FROM aggregated WHERE aggregated.folder_id = folders.id), 0),
			download_count = COALESCE((SELECT aggregated.download_count FROM aggregated WHERE aggregated.folder_id = folders.id), 0)
	`
	return tx.Exec(query, ResourceStatusActive, ResourceStatusActive, ResourceStatusActive).Error
}

func RebuildDashboardStatsTx(tx *gorm.DB) error {
	if tx == nil {
		return nil
	}

	nowExpr := "CURRENT_TIMESTAMP"

	if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&DailyStat{}).Error; err != nil {
		return err
	}
	if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&SystemStat{}).Error; err != nil {
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
	`, GlobalSystemStatsKey, ResourceStatusActive, SubmissionStatusPending, ReportStatusPending).Error; err != nil {
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
	`, ResourceStatusActive).Error; err != nil {
		return err
	}

	return nil
}
