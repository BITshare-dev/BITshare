package bootstrap

import (
	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func rebuildFolderStats(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Model(&model.Folder{}).Updates(map[string]any{
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
		return tx.Exec(query, model.ResourceStatusActive, model.ResourceStatusActive, model.ResourceStatusActive).Error
	})
}
