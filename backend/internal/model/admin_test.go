package model

import "testing"

func TestNormalizeAdminPermissions(t *testing.T) {
	normalized := NormalizeAdminPermissions([]AdminPermission{
		AdminPermissionManageTags,
		AdminPermissionReviewSubmissions,
		AdminPermissionManageTags,
		AdminPermission("invalid"),
	})

	expected := "manage_tags,review_submissions"
	if normalized != expected {
		t.Fatalf("expected normalized permissions %q, got %q", expected, normalized)
	}
}

func TestParseAdminPermissions(t *testing.T) {
	permissions := ParseAdminPermissions("manage_tags, review_submissions,manage_tags,invalid")

	if len(permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %v", permissions)
	}
	if permissions[0] != AdminPermissionManageTags {
		t.Fatalf("expected first permission %q, got %q", AdminPermissionManageTags, permissions[0])
	}
	if permissions[1] != AdminPermissionReviewSubmissions {
		t.Fatalf("expected second permission %q, got %q", AdminPermissionReviewSubmissions, permissions[1])
	}
}

func TestValidateAdminRoleAndStatus(t *testing.T) {
	if err := ValidateAdminRole(string(AdminRoleSuperAdmin)); err != nil {
		t.Fatalf("expected valid super admin role, got %v", err)
	}
	if err := ValidateAdminRole("unknown"); err == nil {
		t.Fatal("expected invalid role error")
	}

	if err := ValidateAdminStatus(AdminStatusActive); err != nil {
		t.Fatalf("expected valid active status, got %v", err)
	}
	if err := ValidateAdminStatus(AdminStatus("archived")); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestDefaultAdminPermissions(t *testing.T) {
	adminPermissions := DefaultAdminPermissions(AdminRoleAdmin)
	if len(adminPermissions) != 1 || adminPermissions[0] != AdminPermissionReviewSubmissions {
		t.Fatalf("expected default admin permission set to contain review_submissions, got %v", adminPermissions)
	}

	superAdminPermissions := DefaultAdminPermissions(AdminRoleSuperAdmin)
	if len(superAdminPermissions) != 0 {
		t.Fatalf("expected super admin explicit permission list to be empty, got %v", superAdminPermissions)
	}
}

func TestAdminHasPermission(t *testing.T) {
	admin := Admin{
		Role:        string(AdminRoleAdmin),
		Permissions: NormalizeAdminPermissions([]AdminPermission{AdminPermissionManageTags}),
		Status:      AdminStatusActive,
	}

	if !admin.HasPermission(AdminPermissionManageTags) {
		t.Fatal("expected admin to have manage_tags permission")
	}
	if admin.HasPermission(AdminPermissionManageSystem) {
		t.Fatal("expected admin to not have manage_system permission")
	}

	superAdmin := Admin{
		Role:   string(AdminRoleSuperAdmin),
		Status: AdminStatusActive,
	}
	if !superAdmin.HasPermission(AdminPermissionManageSystem) {
		t.Fatal("expected super admin to bypass permission checks")
	}
}
