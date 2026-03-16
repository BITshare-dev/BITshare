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
	TotalVisitorIPs  int64 `json:"total_visitor_ips"`
	TotalFiles       int64 `json:"total_files"`
	TotalDownloads   int64 `json:"total_downloads"`
	RecentVisitorIPs int64 `json:"recent_visitor_ips"`
	RecentFiles      int64 `json:"recent_files"`
	RecentDownloads  int64 `json:"recent_downloads"`
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
		TotalVisitorIPs:  row.TotalVisitorIPs,
		TotalFiles:       row.TotalFiles,
		TotalDownloads:   row.TotalDownloads,
		RecentVisitorIPs: row.RecentVisitorIPs,
		RecentFiles:      row.RecentFiles,
		RecentDownloads:  row.RecentDownloads,
	}, nil
}
