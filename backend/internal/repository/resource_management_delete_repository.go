package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func (r *ResourceManagementRepository) DeleteFileWithLog(
	ctx context.Context,
	fileID string,
	operatorID string,
	operatorIP string,
	logID string,
	detail string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var file model.File
		if err := tx.Select("id, folder_id, size, download_count, created_at").
			Where("id = ?", fileID).
			Take(&file).Error; err != nil {
			return err
		}

		if err := detachDeletedResourcesTx(tx, []string{fileID}, nil); err != nil {
			return err
		}

		if err := tx.Where("id = ?", fileID).Delete(&model.File{}).Error; err != nil {
			return fmt.Errorf("delete file: %w", err)
		}
		if err := model.AdjustFolderStatsTx(tx, file.FolderID, -file.Size, -file.DownloadCount, -1); err != nil {
			return fmt.Errorf("adjust deleted file folder stats: %w", err)
		}
		if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{
			TotalFiles: -1,
		}); err != nil {
			return fmt.Errorf("adjust deleted file system stats: %w", err)
		}
		if err := model.AdjustDailyStatsTx(tx, file.CreatedAt, model.DailyStatsDelta{NewFiles: -1}); err != nil {
			return fmt.Errorf("adjust deleted file daily stats: %w", err)
		}

		return createOperationLogTx(tx, logID, operatorID, "resource_deleted", "file", fileID, detail, operatorIP, now)
	})
}

func (r *ResourceManagementRepository) DeleteFolderTreeWithLog(
	ctx context.Context,
	rootFolderID string,
	folderIDs []string,
	rootSourcePath string,
	operatorID string,
	operatorIP string,
	logID string,
	detail string,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(folderIDs) == 0 {
			return gorm.ErrRecordNotFound
		}

		var root model.Folder
		if err := tx.Select("id, parent_id, file_count, total_size, download_count").
			Where("id = ?", rootFolderID).
			Take(&root).Error; err != nil {
			return fmt.Errorf("load root folder stats: %w", err)
		}

		type fileDayStat struct {
			Day   string
			Count int64
		}
		var activeFileDayStats []fileDayStat
		if err := tx.Model(&model.File{}).
			Select("DATE(created_at) AS day, COUNT(*) AS count").
			Where("folder_id IN ?", folderIDs).
			Group("DATE(created_at)").
			Scan(&activeFileDayStats).Error; err != nil {
			return fmt.Errorf("load deleted folder tree daily file stats: %w", err)
		}

		var fileIDs []string
		if err := tx.Model(&model.File{}).
			Where("folder_id IN ?", folderIDs).
			Pluck("id", &fileIDs).Error; err != nil {
			return fmt.Errorf("list deleted folder tree files: %w", err)
		}

		if err := detachDeletedResourcesTx(tx, fileIDs, folderIDs); err != nil {
			return err
		}

		if err := tx.Where("folder_id IN ?", folderIDs).Delete(&model.File{}).Error; err != nil {
			return fmt.Errorf("delete folder tree files: %w", err)
		}

		if err := tx.Where("id IN ?", folderIDs).Delete(&model.Folder{}).Error; err != nil {
			return fmt.Errorf("delete folder tree: %w", err)
		}

		if root.ParentID != nil {
			if err := model.AdjustFolderStatsTx(tx, root.ParentID, -root.TotalSize, -root.DownloadCount, -root.FileCount); err != nil {
				return fmt.Errorf("adjust ancestor folder stats: %w", err)
			}
		}
		if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{
			TotalFiles: -root.FileCount,
		}); err != nil {
			return fmt.Errorf("adjust deleted folder tree system stats: %w", err)
		}
		for _, stat := range activeFileDayStats {
			dayTime, err := time.Parse("2006-01-02", stat.Day)
			if err != nil {
				return fmt.Errorf("parse deleted folder tree day: %w", err)
			}
			if err := model.AdjustDailyStatsTx(tx, dayTime, model.DailyStatsDelta{NewFiles: -stat.Count}); err != nil {
				return fmt.Errorf("adjust deleted folder tree daily stats: %w", err)
			}
		}

		return createOperationLogTx(tx, logID, operatorID, "resource_deleted", "folder", rootFolderID, detail, operatorIP, now)
	})
}
