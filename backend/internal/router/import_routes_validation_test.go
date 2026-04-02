package router

import (
	"net/http"
	"path/filepath"
	"testing"

	"openshare/backend/internal/model"
)

func TestImportLocalDirectoryRejectsDuplicateAndChildDirectory(t *testing.T) {
	_, _, cookie, engine, _ := newImportRouteEnv(t, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)

	recorder := importLocalDirectory(t, engine, cookie, importRoot)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	duplicateRecorder := importLocalDirectory(t, engine, cookie, importRoot)
	if duplicateRecorder.Code != http.StatusConflict {
		t.Fatalf("expected duplicate import status 409, got %d body=%s", duplicateRecorder.Code, duplicateRecorder.Body.String())
	}

	childPath := filepath.Join(importRoot, "nested")
	childRecorder := importLocalDirectory(t, engine, cookie, childPath)
	if childRecorder.Code != http.StatusConflict {
		t.Fatalf("expected child import status 409, got %d body=%s", childRecorder.Code, childRecorder.Body.String())
	}
}

func TestImportLocalDirectoryRejectsParentDirectoryOfManagedRoot(t *testing.T) {
	_, _, cookie, engine, _ := newImportRouteEnv(t, adminAccess{
		username: "sysadmin",
		password: "s3cret-pass",
		role:     string(model.AdminRoleAdmin),
		permissions: []model.AdminPermission{
			model.AdminPermissionManageSystem,
		},
	})
	importRoot := createImportFixture(t)
	childPath := filepath.Join(importRoot, "nested")

	childRecorder := importLocalDirectory(t, engine, cookie, childPath)
	if childRecorder.Code != http.StatusOK {
		t.Fatalf("expected child import status 200, got %d body=%s", childRecorder.Code, childRecorder.Body.String())
	}

	parentRecorder := importLocalDirectory(t, engine, cookie, importRoot)
	if parentRecorder.Code != http.StatusConflict {
		t.Fatalf("expected parent import status 409, got %d body=%s", parentRecorder.Code, parentRecorder.Body.String())
	}
}
