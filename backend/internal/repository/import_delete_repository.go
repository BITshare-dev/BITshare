package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type ManagedRootUnmanageResult struct {
	PendingStagingPaths []string
}

type dateCountStat struct {
	Day   string
	Count int64
}

type managedRootCleanupStats struct {
	PendingStagingPaths []string
	PendingSubmissions  int64
	PendingFeedbacks    int64
}

func (r *ImportRepository) UnmanageManagedRootWithLog(ctx context.Context, rootFolderID, operatorID, operatorIP, detail, logID string, now time.Time) (*ManagedRootUnmanageResult, error) {
	result := &ManagedRootUnmanageResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var folders []FolderTreeFolderRow
		if err := tx.Model(&model.Folder{}).
			Select("id, parent_id, name, source_path").
			Find(&folders).Error; err != nil {
			return fmt.Errorf("list folders for deletion: %w", err)
		}

		childrenByParent := make(map[string][]string)
		folderByID := make(map[string]FolderTreeFolderRow, len(folders))
		for _, folder := range folders {
			folderByID[folder.ID] = folder
			if folder.ParentID != nil {
				childrenByParent[*folder.ParentID] = append(childrenByParent[*folder.ParentID], folder.ID)
			}
		}

		root, ok := folderByID[rootFolderID]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		if root.ParentID != nil {
			return ErrManagedRootRequired
		}

		folderIDs := []string{rootFolderID}
		queue := []string{rootFolderID}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			children := childrenByParent[current]
			folderIDs = append(folderIDs, children...)
			queue = append(queue, children...)
		}

		var fileIDs []string
		if err := tx.Model(&model.File{}).Where("folder_id IN ?", folderIDs).Pluck("id", &fileIDs).Error; err != nil {
			return fmt.Errorf("list files for deletion: %w", err)
		}

		type activeFileAggregate struct {
			Count int64
		}
		var activeFiles activeFileAggregate
		if err := tx.Model(&model.File{}).
			Select("COUNT(*) AS count").
			Where("folder_id IN ?", folderIDs).
			Scan(&activeFiles).Error; err != nil {
			return fmt.Errorf("aggregate files for deletion: %w", err)
		}

		var createdDayStats []dateCountStat
		if err := tx.Model(&model.File{}).
			Select("DATE(created_at) AS day, COUNT(*) AS count").
			Where("folder_id IN ?", folderIDs).
			Group("DATE(created_at)").
			Scan(&createdDayStats).Error; err != nil {
			return fmt.Errorf("aggregate file day stats for deletion: %w", err)
		}

		cleanupStats, err := collectManagedRootCleanupStatsTx(tx, fileIDs, folderIDs)
		if err != nil {
			return err
		}
		result.PendingStagingPaths = cleanupStats.PendingStagingPaths

		if err := linkedManagedResourceScope(tx, fileIDs, folderIDs).Delete(&model.Submission{}).Error; err != nil {
			return fmt.Errorf("delete submissions: %w", err)
		}
		if err := linkedManagedResourceScope(tx, fileIDs, folderIDs).Delete(&model.Feedback{}).Error; err != nil {
			return fmt.Errorf("delete feedbacks: %w", err)
		}
		if len(fileIDs) > 0 {
			if err := tx.Where("folder_id IN ?", folderIDs).Delete(&model.File{}).Error; err != nil {
				return fmt.Errorf("delete files: %w", err)
			}
		}

		if err := tx.Where("id IN ?", folderIDs).Delete(&model.Folder{}).Error; err != nil {
			return fmt.Errorf("delete folders: %w", err)
		}

		if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{
			TotalFiles: -activeFiles.Count,
		}); err != nil {
			return fmt.Errorf("adjust deleted managed root system stats: %w", err)
		}
		for _, stat := range createdDayStats {
			dayTime, err := time.Parse("2006-01-02", stat.Day)
			if err != nil {
				return fmt.Errorf("parse deleted file day: %w", err)
			}
			if err := model.AdjustDailyStatsTx(tx, dayTime, model.DailyStatsDelta{NewFiles: -stat.Count}); err != nil {
				return fmt.Errorf("adjust deleted file daily stats: %w", err)
			}
		}

		if cleanupStats.PendingSubmissions > 0 || cleanupStats.PendingFeedbacks > 0 {
			if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{
				PendingSubmissions: -cleanupStats.PendingSubmissions,
				PendingFeedbacks:   -cleanupStats.PendingFeedbacks,
			}); err != nil {
				return fmt.Errorf("adjust unmanaged directory system stats: %w", err)
			}
		}

		return createOperationLogTx(tx, logID, operatorID, "managed_directory_unmanaged", "folder", rootFolderID, detail, operatorIP, now)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func collectManagedRootCleanupStatsTx(tx *gorm.DB, fileIDs []string, folderIDs []string) (*managedRootCleanupStats, error) {
	stats := &managedRootCleanupStats{}

	if err := linkedManagedResourceScope(tx.Model(&model.Submission{}), fileIDs, folderIDs).
		Where("status = ?", model.SubmissionStatusPending).
		Distinct("id").
		Count(&stats.PendingSubmissions).Error; err != nil {
		return nil, fmt.Errorf("count pending submissions: %w", err)
	}
	if err := linkedManagedResourceScope(tx.Model(&model.Submission{}), fileIDs, folderIDs).
		Where("status = ?", model.SubmissionStatusPending).
		Where("TRIM(staging_path) <> ''").
		Distinct("staging_path").
		Pluck("staging_path", &stats.PendingStagingPaths).Error; err != nil {
		return nil, fmt.Errorf("list pending staging paths: %w", err)
	}
	if err := linkedManagedResourceScope(tx.Model(&model.Feedback{}), fileIDs, folderIDs).
		Where("status = ?", model.FeedbackStatusPending).
		Distinct("id").
		Count(&stats.PendingFeedbacks).Error; err != nil {
		return nil, fmt.Errorf("count pending feedbacks: %w", err)
	}

	return stats, nil
}

func linkedManagedResourceScope(tx *gorm.DB, fileIDs []string, folderIDs []string) *gorm.DB {
	hasCondition := false
	if len(folderIDs) > 0 {
		tx = tx.Where("folder_id IN ?", folderIDs)
		hasCondition = true
	}
	if len(fileIDs) > 0 {
		if hasCondition {
			tx = tx.Or("file_id IN ?", fileIDs)
		} else {
			tx = tx.Where("file_id IN ?", fileIDs)
			hasCondition = true
		}
	}
	if !hasCondition {
		return tx.Where("1 = 0")
	}
	return tx
}
