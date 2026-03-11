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
	&model.Tag{},
	&model.FileTag{},
	&model.FolderTag{},
	&model.Report{},
	&model.Announcement{},
	&model.OperationLog{},
	&model.AdminSession{},
	&model.TagSubmission{},
}

// EnsureSchema initializes the current baseline schema used by the application.
func EnsureSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(managedModels...); err != nil {
		return fmt.Errorf("auto migrate schema: %w", err)
	}

	return nil
}
