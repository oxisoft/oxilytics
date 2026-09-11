// Package permissions holds pure authorisation rules. No I/O.
package permissions

import "github.com/oxisoft/oxilytics/internal/models"

type Action string

const (
	ViewData        Action = "view_data"
	ViewSync        Action = "view_sync"
	RunSync         Action = "run_sync"
	EditSettings    Action = "edit_settings"
	ManageUsers     Action = "manage_users"
	ManageProducts  Action = "manage_products"
	IgnoreApps      Action = "ignore_apps"
	ViewIgnoredApps Action = "view_ignored_apps"
	ViewSetup       Action = "view_setup"
	TestStore       Action = "test_store"
	ManageProfile   Action = "manage_profile"
)

// Can reports whether a user may perform an action. Disabled users can do nothing.
func Can(u *models.User, a Action) bool {
	if u == nil || u.Disabled {
		return false
	}
	switch u.Role {
	case models.RoleAdmin:
		return true
	case models.RoleViewer:
		switch a {
		case ViewData, ViewSync, ViewSetup, ManageProfile:
			return true
		}
	}
	return false
}

// CanDeleteUser: admins only, never self, never the last active admin.
func CanDeleteUser(actor, target *models.User, activeAdmins int) bool {
	if !Can(actor, ManageUsers) || actor.ID == target.ID {
		return false
	}
	if target.Role == models.RoleAdmin && !target.Disabled && activeAdmins <= 1 {
		return false
	}
	return true
}

// CanDemoteOrDisable guards role/disabled changes with the same last-admin rule.
func CanDemoteOrDisable(actor, target *models.User, newRole models.Role, newDisabled bool, activeAdmins int) bool {
	if !Can(actor, ManageUsers) {
		return false
	}
	losesAdmin := target.Role == models.RoleAdmin && !target.Disabled && (newRole != models.RoleAdmin || newDisabled)
	if losesAdmin && activeAdmins <= 1 {
		return false
	}
	if actor.ID == target.ID && losesAdmin {
		return false
	}
	return true
}
