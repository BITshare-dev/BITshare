package session

import "openshare/backend/internal/model"

func (i AdminIdentity) IsSuperAdmin() bool {
	return i.Role == string(model.AdminRoleSuperAdmin)
}

func (i AdminIdentity) HasPermission(permission model.AdminPermission) bool {
	if i.IsSuperAdmin() {
		return true
	}

	for _, granted := range i.Permissions {
		if granted == permission {
			return true
		}
	}

	return false
}
