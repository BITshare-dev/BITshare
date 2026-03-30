package bootstrap

import (
	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func rebuildDashboardStats(db *gorm.DB) error {
	return db.Transaction(model.RebuildDashboardStatsTx)
}
