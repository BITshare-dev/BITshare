package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/config"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

const systemPolicyKey = "system_policy"

type UploadPolicy struct {
	MaxUploadTotalBytes int64 `json:"max_upload_total_bytes"`
}

func (p *UploadPolicy) UnmarshalJSON(data []byte) error {
	type uploadPolicyAlias struct {
		MaxUploadTotalBytes int64 `json:"max_upload_total_bytes"`
		MaxFileSizeBytes    int64 `json:"max_file_size_bytes"`
	}

	var raw uploadPolicyAlias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.MaxUploadTotalBytes = raw.MaxUploadTotalBytes
	if p.MaxUploadTotalBytes <= 0 {
		p.MaxUploadTotalBytes = raw.MaxFileSizeBytes
	}
	return nil
}

type SystemPolicy struct {
	Upload UploadPolicy `json:"upload"`
}

type SystemSettingService struct {
	repo          *repository.SystemSettingRepository
	defaultPolicy SystemPolicy
	nowFunc       func() time.Time
}

func defaultSystemPolicy(cfg config.UploadConfig) SystemPolicy {
	return SystemPolicy{
		Upload: UploadPolicy{
			MaxUploadTotalBytes: cfg.MaxUploadTotalBytes,
		},
	}
}

func NewSystemSettingService(repo *repository.SystemSettingRepository, cfg config.Config) *SystemSettingService {
	return &SystemSettingService{
		repo:          repo,
		defaultPolicy: defaultSystemPolicy(cfg.Upload),
		nowFunc:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *SystemSettingService) GetPolicy(ctx context.Context) (*SystemPolicy, error) {
	item, err := s.repo.FindByKey(ctx, systemPolicyKey)
	if err != nil {
		return nil, err
	}
	if item == nil || strings.TrimSpace(item.Value) == "" {
		policy := s.defaultPolicy
		return &policy, nil
	}

	var policy SystemPolicy
	if err := json.Unmarshal([]byte(item.Value), &policy); err != nil {
		return nil, fmt.Errorf("decode system policy: %w", err)
	}
	return &policy, nil
}

func (s *SystemSettingService) SavePolicy(ctx context.Context, policy SystemPolicy, operatorID string, operatorIP string) (*SystemPolicy, error) {
	if policy.Upload.MaxUploadTotalBytes <= 0 {
		return nil, ErrInvalidUploadInput
	}

	payload, err := json.Marshal(policy)
	if err != nil {
		return nil, fmt.Errorf("encode system policy: %w", err)
	}
	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate system policy log id: %w", err)
	}
	if err := s.repo.UpsertWithLog(ctx, systemPolicyKey, string(payload), operatorID, operatorIP, logID, s.nowFunc()); err != nil {
		return nil, fmt.Errorf("save system policy: %w", err)
	}
	return &policy, nil
}
