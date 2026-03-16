package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

var (
	ErrAdminNotFound        = errors.New("admin not found")
	ErrAdminInvalidInput    = errors.New("invalid admin input")
	ErrAdminImmutableTarget = errors.New("admin target cannot be modified")
	ErrAdminDeleteDenied    = errors.New("admin cannot be deleted")
)

type AdminManagementService struct {
	repo    *repository.AdminRepository
	nowFunc func() time.Time
}

type ManagedAdminItem struct {
	ID          string                  `json:"id"`
	Username    string                  `json:"username"`
	DisplayName string                  `json:"display_name"`
	AvatarURL   string                  `json:"avatar_url"`
	Role        string                  `json:"role"`
	Status      model.AdminStatus       `json:"status"`
	Permissions []model.AdminPermission `json:"permissions"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type CreateAdminInput struct {
	Permissions []model.AdminPermission
	OperatorID  string
	OperatorIP  string
}

type CreatedAdminResult struct {
	Item        ManagedAdminItem `json:"item"`
	LoginID     string           `json:"login_id"`
	Password    string           `json:"password"`
	DisplayName string           `json:"display_name"`
}

type UpdateAdminInput struct {
	Status      model.AdminStatus
	Permissions []model.AdminPermission
	OperatorID  string
	OperatorIP  string
}

type ResetAdminPasswordInput struct {
	NewPassword string
	OperatorID  string
	OperatorIP  string
}

func NewAdminManagementService(repo *repository.AdminRepository) *AdminManagementService {
	return &AdminManagementService{
		repo:    repo,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
}

func (s *AdminManagementService) ListAdmins(ctx context.Context) ([]ManagedAdminItem, error) {
	admins, err := s.repo.ListAdmins(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ManagedAdminItem, 0, len(admins))
	for _, admin := range admins {
		items = append(items, mapManagedAdmin(admin))
	}
	return items, nil
}

func (s *AdminManagementService) CreateAdmin(ctx context.Context, input CreateAdminInput) (*CreatedAdminResult, error) {
	if len(input.Permissions) == 0 {
		return nil, ErrAdminInvalidInput
	}

	loginID, err := s.generateUniqueLoginID(ctx)
	if err != nil {
		return nil, err
	}
	password, err := generateAdminPassword()
	if err != nil {
		return nil, fmt.Errorf("generate admin password: %w", err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash admin password: %w", err)
	}
	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate admin id: %w", err)
	}
	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate admin log id: %w", err)
	}
	now := s.nowFunc()
	admin := &model.Admin{
		ID:           id,
		Username:     loginID,
		DisplayName:  loginID,
		PasswordHash: string(hashed),
		Role:         string(model.AdminRoleAdmin),
		Permissions:  model.NormalizeAdminPermissions(input.Permissions),
		Status:       model.AdminStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateWithLog(ctx, admin, input.OperatorID, input.OperatorIP, "admin_created", loginID, logID, now); err != nil {
		return nil, fmt.Errorf("create admin: %w", err)
	}
	item := mapManagedAdmin(*admin)
	return &CreatedAdminResult{
		Item:        item,
		LoginID:     loginID,
		Password:    password,
		DisplayName: loginID,
	}, nil
}

func (s *AdminManagementService) UpdateAdmin(ctx context.Context, adminID string, input UpdateAdminInput) (*ManagedAdminItem, error) {
	target, err := s.repo.FindByID(ctx, strings.TrimSpace(adminID))
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrAdminNotFound
	}
	if target.Role == string(model.AdminRoleSuperAdmin) {
		return nil, ErrAdminImmutableTarget
	}
	if model.ValidateAdminStatus(input.Status) != nil {
		return nil, ErrAdminInvalidInput
	}

	now := s.nowFunc()
	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate admin log id: %w", err)
	}
	if err := s.repo.UpdateAdminWithLog(ctx, target.ID, map[string]any{
		"status":      input.Status,
		"permissions": model.NormalizeAdminPermissions(input.Permissions),
		"updated_at":  now,
	}, input.OperatorID, input.OperatorIP, "admin_updated", target.Username, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdminNotFound
		}
		return nil, fmt.Errorf("update admin: %w", err)
	}

	updated, err := s.repo.FindByID(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	item := mapManagedAdmin(*updated)
	return &item, nil
}

func (s *AdminManagementService) ResetPassword(ctx context.Context, adminID string, input ResetAdminPasswordInput) error {
	target, err := s.repo.FindByID(ctx, strings.TrimSpace(adminID))
	if err != nil {
		return err
	}
	if target == nil {
		return ErrAdminNotFound
	}
	if target.Role == string(model.AdminRoleSuperAdmin) {
		return ErrAdminImmutableTarget
	}
	if len(input.NewPassword) < 8 {
		return ErrAdminInvalidInput
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	now := s.nowFunc()
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate admin log id: %w", err)
	}
	if err := s.repo.UpdateAdminWithLog(ctx, target.ID, map[string]any{
		"password_hash": string(hashed),
		"updated_at":    now,
	}, input.OperatorID, input.OperatorIP, "admin_password_reset", target.Username, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAdminNotFound
		}
		return fmt.Errorf("reset admin password: %w", err)
	}
	return nil
}

func (s *AdminManagementService) DeleteAdmin(ctx context.Context, adminID, operatorID, operatorIP string) error {
	target, err := s.repo.FindByID(ctx, strings.TrimSpace(adminID))
	if err != nil {
		return err
	}
	if target == nil {
		return ErrAdminNotFound
	}
	if target.Role == string(model.AdminRoleSuperAdmin) {
		return ErrAdminImmutableTarget
	}
	if strings.TrimSpace(target.ID) == strings.TrimSpace(operatorID) {
		return ErrAdminDeleteDenied
	}

	now := s.nowFunc()
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate admin delete log id: %w", err)
	}
	if err := s.repo.DeleteAdminWithLog(ctx, target.ID, operatorID, operatorIP, "admin_deleted", target.Username, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAdminNotFound
		}
		return fmt.Errorf("delete admin: %w", err)
	}
	return nil
}

func (s *AdminManagementService) generateUniqueLoginID(ctx context.Context) (string, error) {
	for range 200 {
		loginID, err := generateAdminLoginID()
		if err != nil {
			return "", fmt.Errorf("generate login id: %w", err)
		}
		exists, err := s.repo.UsernameExists(ctx, loginID)
		if err != nil {
			return "", err
		}
		if !exists {
			return loginID, nil
		}
	}
	return "", ErrAdminInvalidInput
}

func generateAdminLoginID() (string, error) {
	value, err := randomInt(10000)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("1%04d", value), nil
}

func generateAdminPassword() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buffer := make([]byte, 8)
	for index := range buffer {
		offset, err := randomInt(len(alphabet))
		if err != nil {
			return "", err
		}
		buffer[index] = alphabet[offset]
	}
	return string(buffer), nil
}

func randomInt(max int) (int, error) {
	if max <= 0 {
		return 0, ErrAdminInvalidInput
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func mapManagedAdmin(admin model.Admin) ManagedAdminItem {
	return ManagedAdminItem{
		ID:          admin.ID,
		Username:    admin.Username,
		DisplayName: admin.DisplayName,
		AvatarURL:   admin.AvatarURL,
		Role:        admin.Role,
		Status:      admin.Status,
		Permissions: admin.PermissionList(),
		CreatedAt:   admin.CreatedAt,
		UpdatedAt:   admin.UpdatedAt,
	}
}
