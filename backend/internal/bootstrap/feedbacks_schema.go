package bootstrap

import (
	"fmt"

	"gorm.io/gorm"
)

func migrateFeedbacksSchema(db *gorm.DB) error {
	if db.Migrator().HasTable("feedbacks") || !db.Migrator().HasTable("reports") {
		return nil
	}
	if err := db.Exec(`ALTER TABLE reports RENAME TO feedbacks`).Error; err != nil {
		return fmt.Errorf("rename legacy reports table: %w", err)
	}
	return nil
}
