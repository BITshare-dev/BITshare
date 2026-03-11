package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"openshare/backend/internal/bootstrap"
	"openshare/backend/internal/config"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/session"
	"openshare/backend/pkg/database"
	"openshare/backend/pkg/identity"
)

func TestAdminLoginCreatesSessionAndReturnsProfile(t *testing.T) {
	db := newRouterTestDB(t)
	admin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	body := bytes.NewBufferString(`{"username":"superadmin","password":"s3cret-pass"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/session/login", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Admin struct {
			ID          string   `json:"id"`
			Username    string   `json:"username"`
			Role        string   `json:"role"`
			Status      string   `json:"status"`
			Permissions []string `json:"permissions"`
		} `json:"admin"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if response.Admin.ID != admin.ID {
		t.Fatalf("expected admin id %q, got %q", admin.ID, response.Admin.ID)
	}
	if response.Admin.Username != admin.Username {
		t.Fatalf("expected username %q, got %q", admin.Username, response.Admin.Username)
	}
	if len(response.Admin.Permissions) != 0 {
		t.Fatalf("expected no explicit permissions for super admin bootstrap test, got %v", response.Admin.Permissions)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "openshare_session" {
		t.Fatalf("unexpected cookie name %q", cookies[0].Name)
	}

	var count int64
	if err := db.Model(&model.AdminSession{}).Count(&count).Error; err != nil {
		t.Fatalf("count sessions failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 persisted session, got %d", count)
	}
}

func TestAdminLoginRejectsInvalidCredentials(t *testing.T) {
	db := newRouterTestDB(t)
	createRouterTestAdmin(t, db, "superadmin", "correct-password")
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	body := bytes.NewBufferString(`{"username":"superadmin","password":"wrong-password"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/session/login", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.AdminSession{}).Count(&count).Error; err != nil {
		t.Fatalf("count sessions failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 persisted sessions, got %d", count)
	}
}

func TestAdminLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	db := newRouterTestDB(t)
	admin := createRouterTestAdmin(t, db, "superadmin", "s3cret-pass")
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	cookieValue, identity, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if identity.AdminID != admin.ID {
		t.Fatalf("expected admin id %q, got %q", admin.ID, identity.AdminID)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/session/logout", nil)
	request.AddCookie(&http.Cookie{
		Name:  manager.CookieName(),
		Value: cookieValue,
		Path:  "/",
	})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", recorder.Code)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie in logout response, got %d", len(cookies))
	}
	if cookies[0].MaxAge != -1 {
		t.Fatalf("expected cleared cookie MaxAge=-1, got %d", cookies[0].MaxAge)
	}

	var count int64
	if err := db.Model(&model.AdminSession{}).Count(&count).Error; err != nil {
		t.Fatalf("count sessions failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 persisted sessions after logout, got %d", count)
	}
}

func TestAdminMeRequiresAuthentication(t *testing.T) {
	db := newRouterTestDB(t)
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	request := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
}

func TestAdminMeReturnsIdentityFromSession(t *testing.T) {
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
			model.AdminPermissionManageTags,
		},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Admin struct {
			ID          string   `json:"id"`
			Username    string   `json:"username"`
			Role        string   `json:"role"`
			Permissions []string `json:"permissions"`
		} `json:"admin"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if response.Admin.ID != admin.ID {
		t.Fatalf("expected admin id %q, got %q", admin.ID, response.Admin.ID)
	}
	if response.Admin.Role != string(model.AdminRoleAdmin) {
		t.Fatalf("expected role %q, got %q", model.AdminRoleAdmin, response.Admin.Role)
	}
	if len(response.Admin.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %v", response.Admin.Permissions)
	}
}

func TestPermissionMiddlewareRejectsUnauthorizedPermission(t *testing.T) {
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/_internal/system", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPermissionMiddlewareAllowsGrantedPermission(t *testing.T) {
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username: "reviewer",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionReviewSubmissions,
		},
	})
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/_internal/review", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPermissionMiddlewareAllowsSuperAdminBypass(t *testing.T) {
	db := newRouterTestDB(t)
	admin := createRouterTestAdminWithAccess(t, db, adminAccess{
		username:    "superadmin",
		password:    "s3cret-pass",
		role:        string(model.AdminRoleSuperAdmin),
		permissions: nil,
	})
	manager := newRouterSessionManager(db)
	engine := New(db, manager)

	cookieValue, _, err := manager.Create(t.Context(), admin)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/_internal/system", nil)
	request.AddCookie(&http.Cookie{Name: manager.CookieName(), Value: cookieValue, Path: "/"})
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func newRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "openshare-router-test.db")
	db, err := database.NewSQLite(database.Options{
		Path:      dbPath,
		LogLevel:  "silent",
		EnableWAL: true,
		Pragmas: []database.Pragma{
			{Name: "foreign_keys", Value: "ON"},
			{Name: "busy_timeout", Value: "5000"},
		},
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}

	if err := bootstrap.EnsureSchema(db); err != nil {
		t.Fatalf("ensure schema failed: %v", err)
	}

	return db
}

func newRouterSessionManager(db *gorm.DB) *session.Manager {
	return session.NewManager(db, config.SessionConfig{
		Name:            "openshare_session",
		Secret:          "test-secret",
		Path:            "/",
		MaxAgeSeconds:   3600,
		HTTPOnly:        true,
		Secure:          false,
		SameSite:        "lax",
		RenewWindowSecs: 300,
	}, repository.NewAdminSessionRepository())
}

func createRouterTestAdmin(t *testing.T, db *gorm.DB, username, password string) *model.Admin {
	t.Helper()
	return createRouterTestAdminWithAccess(t, db, adminAccess{
		username: username,
		password: password,
		role:     string(model.AdminRoleSuperAdmin),
	})
}

type adminAccess struct {
	username    string
	password    string
	role        string
	permissions []model.AdminPermission
}

func createRouterTestAdminWithAccess(t *testing.T, db *gorm.DB, access adminAccess) *model.Admin {
	t.Helper()

	adminID, err := identity.NewID()
	if err != nil {
		t.Fatalf("generate admin id failed: %v", err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(access.password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate password hash failed: %v", err)
	}

	admin := &model.Admin{
		ID:           adminID,
		Username:     access.username,
		PasswordHash: string(passwordHash),
		Role:         access.role,
		Permissions:  model.NormalizeAdminPermissions(access.permissions),
		Status:       model.AdminStatusActive,
	}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("create admin failed: %v", err)
	}

	return admin
}
