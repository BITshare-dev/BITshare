package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

var (
	ErrNoSession      = errors.New("session not found")
	ErrInvalidSession = errors.New("session is invalid")
	ErrExpiredSession = errors.New("session is expired")
	ErrInactiveAdmin  = errors.New("admin is not active")
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type Manager struct {
	db         *gorm.DB
	config     config.SessionConfig
	repository *repository.AdminSessionRepository
	clock      Clock
	signingKey []byte
}

type AdminIdentity struct {
	SessionID   string
	AdminID     string
	Username    string
	Role        string
	Status      model.AdminStatus
	Permissions []model.AdminPermission
	ExpiresAt   time.Time
}

type ResolveResult struct {
	Identity    AdminIdentity
	Renewed     bool
	ShouldClear bool
	CookieValue string
}

func NewManager(db *gorm.DB, cfg config.SessionConfig, repo *repository.AdminSessionRepository) *Manager {
	return &Manager{
		db:         db,
		config:     cfg,
		repository: repo,
		clock:      realClock{},
		signingKey: []byte(cfg.Secret),
	}
}

func (m *Manager) Create(ctx context.Context, admin *model.Admin) (string, AdminIdentity, error) {
	now := m.clock.Now()
	token, err := randomToken()
	if err != nil {
		return "", AdminIdentity{}, fmt.Errorf("generate session token: %w", err)
	}

	sessionID, err := identity.NewID()
	if err != nil {
		return "", AdminIdentity{}, fmt.Errorf("generate session id: %w", err)
	}

	sessionModel := &model.AdminSession{
		ID:             sessionID,
		AdminID:        admin.ID,
		TokenHash:      hashToken(token),
		ExpiresAt:      now.Add(time.Duration(m.config.MaxAgeSeconds) * time.Second),
		LastActivityAt: now,
	}

	if err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := m.repository.DeleteExpired(tx, now); err != nil {
			return fmt.Errorf("delete expired sessions: %w", err)
		}
		if err := m.repository.Create(tx, sessionModel); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	}); err != nil {
		return "", AdminIdentity{}, err
	}

	cookieValue := m.signToken(token)
	identity := AdminIdentity{
		SessionID:   sessionModel.ID,
		AdminID:     admin.ID,
		Username:    admin.Username,
		Role:        admin.Role,
		Status:      admin.Status,
		Permissions: admin.PermissionList(),
		ExpiresAt:   sessionModel.ExpiresAt,
	}

	return cookieValue, identity, nil
}

func (m *Manager) Resolve(ctx context.Context, cookieValue string) (*ResolveResult, error) {
	token, err := m.verifySignedToken(cookieValue)
	if err != nil {
		return &ResolveResult{ShouldClear: true}, ErrInvalidSession
	}

	now := m.clock.Now()
	sessionModel, err := m.repository.FindByTokenHash(m.db.WithContext(ctx), hashToken(token))
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	if sessionModel == nil {
		return &ResolveResult{ShouldClear: true}, ErrNoSession
	}
	if sessionModel.ExpiresAt.Before(now) || sessionModel.ExpiresAt.Equal(now) {
		if err := m.repository.DeleteByTokenHash(m.db.WithContext(ctx), sessionModel.TokenHash); err != nil {
			return nil, fmt.Errorf("delete expired session: %w", err)
		}
		return &ResolveResult{ShouldClear: true}, ErrExpiredSession
	}
	if sessionModel.Admin.Status != model.AdminStatusActive {
		return &ResolveResult{ShouldClear: true}, ErrInactiveAdmin
	}

	result := &ResolveResult{
		Identity: AdminIdentity{
			SessionID:   sessionModel.ID,
			AdminID:     sessionModel.Admin.ID,
			Username:    sessionModel.Admin.Username,
			Role:        sessionModel.Admin.Role,
			Status:      sessionModel.Admin.Status,
			Permissions: sessionModel.Admin.PermissionList(),
			ExpiresAt:   sessionModel.ExpiresAt,
		},
	}

	if !m.shouldRenew(sessionModel.ExpiresAt, now) {
		return result, nil
	}

	newExpiry := now.Add(time.Duration(m.config.MaxAgeSeconds) * time.Second)
	if err := m.repository.UpdateActivityAndExpiry(m.db.WithContext(ctx), sessionModel.ID, now, newExpiry); err != nil {
		return nil, fmt.Errorf("renew session: %w", err)
	}

	result.Renewed = true
	result.CookieValue = cookieValue
	result.Identity.ExpiresAt = newExpiry
	return result, nil
}

func (m *Manager) Destroy(ctx context.Context, cookieValue string) error {
	token, err := m.verifySignedToken(cookieValue)
	if err != nil {
		return nil
	}

	if err := m.repository.DeleteByTokenHash(m.db.WithContext(ctx), hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (m *Manager) WriteCookie(w http.ResponseWriter, cookieValue string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.config.Name,
		Value:    cookieValue,
		Path:     m.config.Path,
		HttpOnly: m.config.HTTPOnly,
		Secure:   m.config.Secure,
		SameSite: sameSiteMode(m.config.SameSite),
		Expires:  expiresAt.UTC(),
		MaxAge:   m.config.MaxAgeSeconds,
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.config.Name,
		Value:    "",
		Path:     m.config.Path,
		HttpOnly: m.config.HTTPOnly,
		Secure:   m.config.Secure,
		SameSite: sameSiteMode(m.config.SameSite),
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
	})
}

func (m *Manager) shouldRenew(expiresAt, now time.Time) bool {
	if m.config.RenewWindowSecs == 0 {
		return false
	}

	return expiresAt.Sub(now) <= time.Duration(m.config.RenewWindowSecs)*time.Second
}

func (m *Manager) signToken(token string) string {
	mac := hmac.New(sha256.New, m.signingKey)
	mac.Write([]byte(token))
	signature := hex.EncodeToString(mac.Sum(nil))
	return token + "." + signature
}

func (m *Manager) verifySignedToken(cookieValue string) (string, error) {
	token, signature, ok := strings.Cut(cookieValue, ".")
	if !ok || token == "" || signature == "" {
		return "", ErrInvalidSession
	}

	expected := m.signToken(token)
	if !hmac.Equal([]byte(expected), []byte(cookieValue)) {
		return "", ErrInvalidSession
	}

	return token, nil
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func sameSiteMode(value string) http.SameSite {
	switch strings.ToLower(value) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
