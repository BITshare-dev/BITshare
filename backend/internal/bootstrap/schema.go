package bootstrap

import (
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/search"
)

var managedModels = []any{
	&model.Admin{},
	&model.Folder{},
	&model.File{},
	&model.Submission{},
	&model.Report{},
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
	if err := normalizeReportReviewReasons(db); err != nil {
		return fmt.Errorf("normalize report review reasons: %w", err)
	}

	// Drop old unique index on submissions.receipt_code if it exists.
	// Receipt codes are now shared across multiple submissions (same user session).
	if db.Migrator().HasIndex(&model.Submission{}, "ux_submissions_receipt_code") {
		if err := db.Migrator().DropIndex(&model.Submission{}, "ux_submissions_receipt_code"); err != nil {
			return fmt.Errorf("drop old unique receipt_code index: %w", err)
		}
	}

	// Initialize FTS5 virtual table for full-text search.
	// FTS5 may not be available in all SQLite builds (e.g. pure-Go driver in tests).
	if err := search.EnsureFTS5Schema(db); err != nil {
		if errors.Is(err, search.ErrFTS5Unavailable) {
			log.Println("[WARN] FTS5 module not available — full-text search disabled")
		} else {
			return fmt.Errorf("ensure FTS5 schema: %w", err)
		}
	}

	return nil
}
