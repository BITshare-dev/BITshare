package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

type AdminDashboardRepository struct {
	db *gorm.DB
}

type AdminDashboardStatsRow struct {
	TotalVisits        int64
	TotalFiles         int64
	TotalDownloads     int64
	RecentVisits       int64
	RecentFiles        int64
	RecentDownloads    int64
	PendingSubmissions int64
	PendingFeedbacks   int64
}

func NewAdminDashboardRepository(db *gorm.DB) *AdminDashboardRepository {
	return &AdminDashboardRepository{db: db}
}

func (r *AdminDashboardRepository) GetStats(ctx context.Context, since time.Time) (*AdminDashboardStatsRow, error) {
	row := &AdminDashboardStatsRow{}

	var system model.SystemStat
	if err := r.db.WithContext(ctx).
		Where("key = ?", model.GlobalSystemStatsKey).
		Take(&system).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("load system stats: %w", err)
		}
	}

	row.TotalVisits = system.TotalVisits
	row.TotalFiles = system.TotalFiles
	row.TotalDownloads = system.TotalDownloads
	row.PendingSubmissions = system.PendingSubmissions
	row.PendingFeedbacks = system.PendingFeedbacks

	sinceDay := since.UTC().Format("2006-01-02")
	type dailySums struct {
		RecentFiles     int64
		RecentDownloads int64
		RecentVisits    int64
	}
	var daily dailySums
	if err := r.db.WithContext(ctx).
		Model(&model.DailyStat{}).
		Select("COALESCE(SUM(new_files), 0) AS recent_files, COALESCE(SUM(downloads), 0) AS recent_downloads, COALESCE(SUM(visits), 0) AS recent_visits").
		Where("day >= ?", sinceDay).
		Scan(&daily).Error; err != nil {
		return nil, fmt.Errorf("load recent daily stats: %w", err)
	}
	row.RecentFiles = daily.RecentFiles
	row.RecentDownloads = daily.RecentDownloads
	row.RecentVisits = daily.RecentVisits

	return row, nil
}
