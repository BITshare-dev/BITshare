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

type GuestPolicy struct {
	AllowDirectPublish          bool `json:"allow_direct_publish"`
	ExtraPermissionsEnabled     bool `json:"extra_permissions_enabled"`
	AllowGuestEditTitle         bool `json:"allow_guest_edit_title"`
	AllowGuestEditTags          bool `json:"allow_guest_edit_tags"`
	AllowGuestEditDescription   bool `json:"allow_guest_edit_description"`
	AllowGuestResourceDelete    bool `json:"allow_guest_resource_delete"`
}

type UploadPolicy struct {
	MaxFileSizeBytes  int64    `json:"max_file_size_bytes"`
	MaxTagCount       int      `json:"max_tag_count"`
	AllowedExtensions []string `json:"allowed_extensions"`
}

type SearchPolicy struct {
	EnableFuzzyMatch  bool `json:"enable_fuzzy_match"`
	EnableTagFilter   bool `json:"enable_tag_filter"`
	EnableFolderScope bool `json:"enable_folder_scope"`
	ResultWindow      int  `json:"result_window"`
}

type SystemPolicy struct {
	Guest  GuestPolicy  `json:"guest"`
	Upload UploadPolicy `json:"upload"`
	Search SearchPolicy `json:"search"`
}

type SystemSettingService struct {
	repo          *repository.SystemSettingRepository
	defaultPolicy SystemPolicy
	nowFunc       func() time.Time
}

func defaultSystemPolicy(cfg config.UploadConfig) SystemPolicy {
	return SystemPolicy{
		Guest: GuestPolicy{},
		Upload: UploadPolicy{
			MaxFileSizeBytes:  cfg.MaxFileSizeBytes,
			MaxTagCount:       cfg.MaxTagCount,
			AllowedExtensions: append([]string(nil), cfg.AllowedExtensions...),
		},
		Search: defaultSearchPolicy(),
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
	policy.Search = defaultSearchPolicy()
	return &policy, nil
}

func (s *SystemSettingService) SavePolicy(ctx context.Context, policy SystemPolicy, operatorID string, operatorIP string) (*SystemPolicy, error) {
	if policy.Upload.MaxFileSizeBytes <= 0 || policy.Upload.MaxTagCount < 0 || policy.Search.ResultWindow <= 0 {
		return nil, ErrInvalidUploadInput
	}
	policy.Search = defaultSearchPolicy()

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

func defaultSearchPolicy() SearchPolicy {
	return SearchPolicy{
		EnableFuzzyMatch:  true,
		EnableTagFilter:   true,
		EnableFolderScope: true,
		ResultWindow:      100,
	}
}
