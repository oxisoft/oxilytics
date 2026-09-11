package store

import (
	"context"
	"errors"
	"testing"

	"github.com/oxisoft/oxilytics/internal/models"
)

func TestUsersCRUD(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	n, _ := d.CountUsers(ctx)
	if n != 0 {
		t.Fatalf("expected empty users, got %d", n)
	}

	u := &models.User{Email: " Admin@Example.com ", Name: "Admin", PasswordHash: "h", Role: models.RoleAdmin}
	if err := d.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 || u.Email != "admin@example.com" {
		t.Errorf("create: id=%d email=%q", u.ID, u.Email)
	}

	// duplicate email → conflict
	dup := &models.User{Email: "admin@example.com", Name: "x", PasswordHash: "h", Role: models.RoleViewer}
	if err := d.CreateUser(ctx, dup); !errors.Is(err, ErrConflict) {
		t.Errorf("dup: err=%v want ErrConflict", err)
	}

	got, err := d.GetUserByEmail(ctx, "ADMIN@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID || got.TOTPEnabled {
		t.Errorf("get by email: %+v", got)
	}

	sec, rec := "S", "[]"
	if err := d.UpdateUserTOTP(ctx, u.ID, &sec, &rec); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetUser(ctx, u.ID)
	if !got.TOTPEnabled || *got.TOTPSecret != "S" {
		t.Errorf("totp not stored: %+v", got)
	}

	if err := d.UpdateUser(ctx, u.ID, "Renamed", models.RoleViewer, true); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetUser(ctx, u.ID)
	if got.Name != "Renamed" || got.Role != models.RoleViewer || !got.Disabled {
		t.Errorf("update: %+v", got)
	}

	if err := d.TouchUserLogin(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetUser(ctx, u.ID)
	if got.LastLoginAt == nil {
		t.Error("last_login_at not set")
	}

	list, err := d.ListUsers(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("list: %v %d", err, len(list))
	}

	if err := d.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetUser(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
	if err := d.DeleteUser(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("double delete: %v", err)
	}
}

func TestSettings(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	if err := d.SetSettings(ctx, map[string]string{"sync.schedule.time": "09:00", "new.key": "v"}); err != nil {
		t.Fatal(err)
	}
	s, _ := d.GetSettings(ctx)
	if s["sync.schedule.time"] != "09:00" || s["new.key"] != "v" {
		t.Errorf("settings: %v", s)
	}
}
