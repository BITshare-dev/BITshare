package bootstrap

import (
	"fmt"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

var managedModels = []any{
	&model.Admin{},
	&model.Folder{},
	&model.File{},
	&model.Submission{},
	&model.Feedback{},
	&model.Announcement{},
	&model.OperationLog{},
	&model.AdminSession{},
	&model.SiteVisitEvent{},
	&model.DownloadEvent{},
	&model.SystemSetting{},
	&model.SystemStat{},
	&model.DailyStat{},
}

// EnsureSchema initializes the current baseline schema used by the application.
func EnsureSchema(db *gorm.DB) error {
	if err := rebuildDownloadEventsTableWithoutForeignKey(db); err != nil {
		return fmt.Errorf("rebuild download events schema: %w", err)
	}
	if err := migrateManagedFilesSchema(db); err != nil {
		return fmt.Errorf("migrate files schema: %w", err)
	}
	if err := migrateSubmissionsSchema(db); err != nil {
		return fmt.Errorf("migrate submissions schema: %w", err)
	}
	if err := migrateFeedbacksSchema(db); err != nil {
		return fmt.Errorf("migrate feedbacks schema: %w", err)
	}
	if err := db.AutoMigrate(managedModels...); err != nil {
		return fmt.Errorf("auto migrate schema: %w", err)
	}
	if err := db.Migrator().DropTable("site_visit_daily_uniques", "site_visitors"); err != nil {
		return fmt.Errorf("drop legacy visit tables: %w", err)
	}

	if err := rebuildFolderStats(db); err != nil {
		return fmt.Errorf("rebuild folder stats: %w", err)
	}
	if err := rebuildDashboardStats(db); err != nil {
		return fmt.Errorf("rebuild dashboard stats: %w", err)
	}

	// Drop old unique index on submissions.receipt_code if it exists.
	// Receipt codes are now shared across multiple submissions (same user session).
	if db.Migrator().HasIndex(&model.Submission{}, "ux_submissions_receipt_code") {
		if err := db.Migrator().DropIndex(&model.Submission{}, "ux_submissions_receipt_code"); err != nil {
			return fmt.Errorf("drop old unique receipt_code index: %w", err)
		}
	}

	return nil
}
