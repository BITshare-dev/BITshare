package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/session"
)

var ErrInvalidAdminCredentials = errors.New("invalid admin credentials")

type AdminAuthService struct {
	db             *gorm.DB
	adminRepo      *repository.AdminRepository
	sessionManager *session.Manager
}

type AuthenticatedAdmin struct {
	Admin    *model.Admin
	Identity session.AdminIdentity
	Cookie   string
}

func NewAdminAuthService(
	db *gorm.DB,
	adminRepo *repository.AdminRepository,
	sessionManager *session.Manager,
) *AdminAuthService {
	return &AdminAuthService{
		db:             db,
		adminRepo:      adminRepo,
		sessionManager: sessionManager,
	}
}

func (s *AdminAuthService) Login(ctx context.Context, username, password string) (*AuthenticatedAdmin, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrInvalidAdminCredentials
	}

	admin, err := s.adminRepo.FindByUsername(s.db.WithContext(ctx), username)
	if err != nil {
		return nil, fmt.Errorf("find admin by username: %w", err)
	}
	if admin == nil {
		return nil, ErrInvalidAdminCredentials
	}
	if admin.Status != model.AdminStatusActive {
		return nil, ErrInvalidAdminCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, ErrInvalidAdminCredentials
		}
		return nil, fmt.Errorf("compare admin password: %w", err)
	}

	cookieValue, identity, err := s.sessionManager.Create(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("create admin session: %w", err)
	}

	return &AuthenticatedAdmin{
		Admin:    admin,
		Identity: identity,
		Cookie:   cookieValue,
	}, nil
}

func (s *AdminAuthService) Logout(ctx context.Context, cookieValue string) error {
	if err := s.sessionManager.Destroy(ctx, cookieValue); err != nil {
		return fmt.Errorf("destroy admin session: %w", err)
	}

	return nil
}
