package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/config"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/setup"
	"github.com/oxisoft/oxilytics/internal/store"
)

type client struct {
	t       *testing.T
	srv     *httptest.Server
	cookies []*http.Cookie
}

func newTestServer(t *testing.T, setupRequired bool) (*client, *store.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	cfg := &config.Config{SessionKey: key}
	h := NewRouter(Deps{
		Cfg:      cfg,
		DB:       db,
		Auth:     auth.New(db, key, false, "test"),
		Settings: settings.New(db),
		Setup:    setup.Status{SetupRequired: setupRequired, Stores: map[models.Store]setup.StoreStatus{}},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &client{t: t, srv: srv}, db
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	return c.doBody(method, path, body)
}

func (c *client) doBody(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, &buf)
	req.Header.Set("X-Requested-With", "fetch")
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if cs := resp.Cookies(); len(cs) > 0 {
		c.cookies = cs
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// doRaw decodes into an arbitrary target (arrays etc.), GET only.
func (c *client) doRaw(method, path string, target any) int {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+path, nil)
	req.Header.Set("X-Requested-With", "fetch")
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(target)
	return resp.StatusCode
}

func seedAdmin(t *testing.T, db *store.DB) {
	h, _ := auth.HashPassword("password123")
	if err := db.CreateUser(context.Background(), &models.User{Email: "admin@x.io", Name: "A", Role: models.RoleAdmin, PasswordHash: h}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthFlow(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)

	if code, _ := c.do("GET", "/api/me", nil); code != 401 {
		t.Fatalf("me unauthenticated: %d", code)
	}
	if code, _ := c.do("POST", "/api/auth/login", map[string]string{"email": "admin@x.io", "password": "nope"}); code != 401 {
		t.Fatalf("bad login: %d", code)
	}
	code, out := c.do("POST", "/api/auth/login", map[string]string{"email": "admin@x.io", "password": "password123"})
	if code != 200 || out["user"] == nil {
		t.Fatalf("login: %d %v", code, out)
	}
	code, out = c.do("GET", "/api/me", nil)
	if code != 200 || out["email"] != "admin@x.io" {
		t.Fatalf("me: %d %v", code, out)
	}
	if code, _ := c.do("POST", "/api/auth/logout", nil); code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code, _ := c.do("GET", "/api/me", nil); code != 401 {
		t.Fatalf("me after logout: %d", code)
	}
}

func TestCSRFHeader(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	req, _ := http.NewRequest("POST", c.srv.URL+"/api/auth/login", bytes.NewBufferString(`{"email":"admin@x.io","password":"password123"}`))
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 without header, got %d", resp.StatusCode)
	}
}

func TestUsersAndPermissions(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	c.do("POST", "/api/auth/login", map[string]string{"email": "admin@x.io", "password": "password123"})

	// validation
	code, out := c.do("POST", "/api/users/", map[string]any{"email": "bad", "name": "", "role": "king", "password": "short"})
	if code != 400 || out["error"].(map[string]any)["fields"] == nil {
		t.Fatalf("validation: %d %v", code, out)
	}
	code, out = c.do("POST", "/api/users/", map[string]any{"email": "v@x.io", "name": "V", "role": "viewer", "password": "viewerpass1"})
	if code != 201 {
		t.Fatalf("create viewer: %d %v", code, out)
	}
	vid := int64(out["id"].(float64))

	// last admin guard
	me, _ := db.GetUserByEmail(context.Background(), "admin@x.io")
	code, _ = c.do("PUT", "/api/users/"+itoa(me.ID), map[string]any{"role": "viewer"})
	if code != 409 {
		t.Fatalf("self-demote should be 409, got %d", code)
	}
	code, _ = c.do("DELETE", "/api/users/"+itoa(me.ID), nil)
	if code != 409 {
		t.Fatalf("self-delete should be 409, got %d", code)
	}

	// viewer cannot manage users or settings but can read them (filtered)
	v := &client{t: t, srv: c.srv}
	v.do("POST", "/api/auth/login", map[string]string{"email": "v@x.io", "password": "viewerpass1"})
	if code, _ := v.do("GET", "/api/users/", nil); code != 403 {
		t.Fatalf("viewer list users: %d", code)
	}
	if code, _ := v.do("PUT", "/api/settings", map[string]string{"sync.schedule.time": "10:00"}); code != 403 {
		t.Fatalf("viewer put settings: %d", code)
	}
	code, out = v.do("GET", "/api/settings", nil)
	if code != 200 || out["sync.schedule.time"] != nil || out["ui.default_range_days"] == nil {
		t.Fatalf("viewer settings filtered: %d %v", code, out)
	}

	// admin updates settings with validation
	code, out = c.do("PUT", "/api/settings", map[string]string{"sync.schedule.time": "25:00"})
	if code != 400 {
		t.Fatalf("bad setting: %d %v", code, out)
	}
	code, out = c.do("PUT", "/api/settings", map[string]string{"sync.schedule.time": "10:15"})
	if code != 200 || out["sync.schedule.time"] != "10:15" {
		t.Fatalf("put settings: %d %v", code, out)
	}

	// delete viewer
	if code, _ := c.do("DELETE", "/api/users/"+itoa(vid), nil); code != 204 {
		t.Fatalf("delete viewer: %d", code)
	}
}

func TestSetupGate(t *testing.T) {
	c, db := newTestServer(t, true)
	seedAdmin(t, db)
	code, out := c.do("GET", "/api/health", nil)
	if code != 200 || out["setup_required"] != true {
		t.Fatalf("health: %d %v", code, out)
	}
	c.do("POST", "/api/auth/login", map[string]string{"email": "admin@x.io", "password": "password123"})
	if code, _ := c.do("GET", "/api/setup/status", nil); code != 200 {
		t.Fatalf("setup status: %d", code)
	}
	code, out = c.do("GET", "/api/products", nil)
	if code != 503 || out["error"].(map[string]any)["code"] != "setup_required" {
		t.Fatalf("gate: %d %v", code, out)
	}
	code, _ = c.do("POST", "/api/setup/test/appstore", nil)
	if code != 409 {
		t.Fatalf("test unconfigured store: %d", code)
	}
}

func itoa(n int64) string { return json.Number(fmtInt(n)).String() }

func fmtInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
