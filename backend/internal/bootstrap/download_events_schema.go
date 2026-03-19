package bootstrap

import "gorm.io/gorm"

func rebuildDownloadEventsTableWithoutForeignKey(db *gorm.DB) error {
	if !db.Migrator().HasTable("download_events") {
		return nil
	}

	type foreignKeyRow struct {
		Table string `gorm:"column:table"`
		From  string `gorm:"column:from"`
	}

	var foreignKeys []foreignKeyRow
	if err := db.Raw("PRAGMA foreign_key_list('download_events')").Scan(&foreignKeys).Error; err != nil {
		return err
	}

	hasFileForeignKey := false
	for _, fk := range foreignKeys {
		if fk.Table == "files" && fk.From == "file_id" {
			hasFileForeignKey = true
			break
		}
	}
	if !hasFileForeignKey {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			CREATE TABLE download_events__new (
				id TEXT PRIMARY KEY,
				file_id TEXT NOT NULL,
				created_at DATETIME
			)
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO download_events__new (id, file_id, created_at)
			SELECT id, file_id, created_at
			FROM download_events
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DROP TABLE download_events`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`ALTER TABLE download_events__new RENAME TO download_events`).Error; err != nil {
			return err
		}
		return nil
	})
}
