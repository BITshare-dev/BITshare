package service

import (
	"context"
	"fmt"
	"time"

	"openshare/backend/internal/repository"
)

type AdminDashboardService struct {
	repo    *repository.AdminDashboardRepository
	nowFunc func() time.Time
}

type AdminDashboardStats struct {
	TotalVisits        int64 `json:"total_visits"`
	TotalFiles         int64 `json:"total_files"`
	TotalDownloads     int64 `json:"total_downloads"`
	RecentVisits       int64 `json:"recent_visits"`
	RecentFiles        int64 `json:"recent_files"`
	RecentDownloads    int64 `json:"recent_downloads"`
	PendingSubmissions int64 `json:"pending_submissions"`
	PendingReports     int64 `json:"pending_reports"`
	PendingAuditCount  int64 `json:"pending_audit_count"`
}

func NewAdminDashboardService(repo *repository.AdminDashboardRepository) *AdminDashboardService {
	return &AdminDashboardService{
		repo:    repo,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
}

func (s *AdminDashboardService) GetStats(ctx context.Context) (*AdminDashboardStats, error) {
	since := s.nowFunc().Add(-7 * 24 * time.Hour)
	row, err := s.repo.GetStats(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("load admin dashboard stats: %w", err)
	}

	return &AdminDashboardStats{
		TotalVisits:        row.TotalVisits,
		TotalFiles:         row.TotalFiles,
		TotalDownloads:     row.TotalDownloads,
		RecentVisits:       row.RecentVisits,
		RecentFiles:        row.RecentFiles,
		RecentDownloads:    row.RecentDownloads,
		PendingSubmissions: row.PendingSubmissions,
		PendingReports:     row.PendingReports,
		PendingAuditCount:  row.PendingSubmissions + row.PendingReports,
	}, nil
}
