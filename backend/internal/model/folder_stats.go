package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func AdjustFolderStatsTx(tx *gorm.DB, folderID *string, sizeDelta, downloadDelta, fileCountDelta int64) error {
	if tx == nil || folderID == nil {
		return nil
	}

	currentID := strings.TrimSpace(*folderID)
	if currentID == "" {
		return nil
	}
	if sizeDelta == 0 && downloadDelta == 0 && fileCountDelta == 0 {
		return nil
	}

	visited := make(map[string]struct{})
	for currentID != "" {
		if _, seen := visited[currentID]; seen {
			return fmt.Errorf("detected folder cycle while adjusting stats")
		}
		visited[currentID] = struct{}{}

		var folder Folder
		if err := tx.Model(&Folder{}).
			Select("id, parent_id").
			Where("id = ?", currentID).
			Take(&folder).Error; err != nil {
			return fmt.Errorf("load folder stats target: %w", err)
		}

		updates := map[string]any{}
		if sizeDelta != 0 {
			updates["total_size"] = gorm.Expr("total_size + ?", sizeDelta)
		}
		if downloadDelta != 0 {
			updates["download_count"] = gorm.Expr("download_count + ?", downloadDelta)
		}
		if fileCountDelta != 0 {
			updates["file_count"] = gorm.Expr("file_count + ?", fileCountDelta)
		}
		if len(updates) > 0 {
			if err := tx.Model(&Folder{}).
				Where("id = ?", currentID).
				UpdateColumns(updates).Error; err != nil {
				return fmt.Errorf("adjust folder stats: %w", err)
			}
		}

		if folder.ParentID == nil {
			break
		}
		currentID = strings.TrimSpace(*folder.ParentID)
	}

	return nil
}
