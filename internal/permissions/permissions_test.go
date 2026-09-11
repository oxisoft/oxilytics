package permissions

import (
	"testing"

	"github.com/oxisoft/oxilytics/internal/models"
)

func TestCan(t *testing.T) {
	admin := &models.User{ID: 1, Role: models.RoleAdmin}
	viewer := &models.User{ID: 2, Role: models.RoleViewer}
	disabled := &models.User{ID: 3, Role: models.RoleAdmin, Disabled: true}

	tests := []struct {
		u    *models.User
		a    Action
		want bool
	}{
		{admin, ManageUsers, true},
		{admin, RunSync, true},
		{viewer, ViewData, true},
		{viewer, ViewSync, true},
		{viewer, ManageProfile, true},
		{viewer, RunSync, false},
		{viewer, EditSettings, false},
		{viewer, ViewIgnoredApps, false},
		{viewer, TestStore, false},
		{disabled, ViewData, false},
		{nil, ViewData, false},
	}
	for _, tt := range tests {
		if got := Can(tt.u, tt.a); got != tt.want {
			t.Errorf("Can(%v, %s) = %v, want %v", tt.u, tt.a, got, tt.want)
		}
	}
}

func TestLastAdminGuards(t *testing.T) {
	a1 := &models.User{ID: 1, Role: models.RoleAdmin}
	a2 := &models.User{ID: 2, Role: models.RoleAdmin}
	v := &models.User{ID: 3, Role: models.RoleViewer}

	if CanDeleteUser(a1, a1, 2) {
		t.Error("self delete allowed")
	}
	if CanDeleteUser(a1, a2, 1) {
		t.Error("deleting last admin allowed")
	}
	if !CanDeleteUser(a1, a2, 2) {
		t.Error("deleting one of two admins refused")
	}
	if !CanDeleteUser(a1, v, 1) {
		t.Error("deleting viewer refused")
	}
	if CanDeleteUser(v, a1, 2) {
		t.Error("viewer may delete")
	}

	if CanDemoteOrDisable(a1, a1, models.RoleViewer, false, 2) {
		t.Error("self demote allowed")
	}
	if CanDemoteOrDisable(a1, a2, models.RoleViewer, false, 1) {
		t.Error("demoting last admin allowed")
	}
	if CanDemoteOrDisable(a1, a2, models.RoleAdmin, true, 1) {
		t.Error("disabling last admin allowed")
	}
	if !CanDemoteOrDisable(a1, a2, models.RoleViewer, false, 2) {
		t.Error("demote refused")
	}
	if !CanDemoteOrDisable(a1, v, models.RoleAdmin, false, 1) {
		t.Error("promote refused")
	}
}
