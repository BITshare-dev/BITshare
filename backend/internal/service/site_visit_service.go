package service

import (
	"context"
	"fmt"
	"strings"

	"openshare/backend/internal/repository"
)

type SiteVisitService struct {
	repo *repository.SiteVisitRepository
}

func NewSiteVisitService(repo *repository.SiteVisitRepository) *SiteVisitService {
	return &SiteVisitService{repo: repo}
}

func (s *SiteVisitService) Record(ctx context.Context, scope string, path string, ip string) error {
	if strings.TrimSpace(ip) == "" {
		return nil
	}
	if err := s.repo.Create(ctx, scope, path, ip); err != nil {
		return fmt.Errorf("record site visit: %w", err)
	}
	return nil
}
