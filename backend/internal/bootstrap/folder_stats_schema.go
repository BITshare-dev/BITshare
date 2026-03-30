package bootstrap

import (
	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func rebuildFolderStats(db *gorm.DB) error {
	return db.Transaction(model.RebuildFolderStatsTx)
}
