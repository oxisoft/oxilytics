package httpapi

import (
	"context"
	"testing"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/models"
)

// The capability is opt-in: a token created without it must be refused the
// sync endpoint exactly like any other write.
func TestReadOnlyTokenCannotStartSync(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintTokenWith(t, c, "reporting", false)
	c2 := &client{t: t, srv: c.srv}

	code, out := c2.withToken("POST", "/api/sync/runs", tok, map[string]string{
		"store": "appstore", "mode": "delta",
	})
	if code != 403 {
		t.Fatalf("read-only token starting a sync: want 403, got %d %v", code, out)
	}
	if errObj, _ := out["error"].(map[string]any); errObj["code"] != "read_only_token" {
		t.Fatalf("want read_only_token, got %v", out)
	}
}

// A granted token reaches the handler. The store is unconfigured in tests, so
// a 409 store_not_configured proves the request passed every auth layer and
// was refused for a business reason instead.
func TestSyncTokenReachesTheSyncHandler(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintTokenWith(t, c, "nightly", true)
	c2 := &client{t: t, srv: c.srv}

	code, out := c2.withToken("POST", "/api/sync/runs", tok, map[string]string{
		"store": "appstore", "mode": "delta",
	})
	if code == 403 {
		t.Fatalf("granted token was still refused by the auth layer: %v", out)
	}
	// Any non-403 proves the request cleared every auth check and reached the
	// handler. 201 is a started run; 409 is a configured store that is busy or
	// unconfigured; 501 is this test server, which mounts no sync engine.
	switch code {
	case 201, 409, 501:
	default:
		t.Fatalf("unexpected status %d: %v", code, out)
	}
}

// The capability names one endpoint. Neighbouring sync writes must stay shut,
// or "can start a sync" would quietly mean "can control syncing".
func TestSyncTokenCannotCancelOrReset(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintTokenWith(t, c, "nightly", true)
	c2 := &client{t: t, srv: c.srv}

	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/sync/runs/1/cancel"},
		{"POST", "/api/sync/reset"},
	} {
		code, out := c2.withToken(tc.method, tc.path, tok, nil)
		if code != 403 {
			t.Fatalf("%s %s: want 403, got %d %v", tc.method, tc.path, code, out)
		}
	}
}

// A Bearer token is never attached automatically by a browser, so the CSRF
// header is meaningless for token callers. Requiring it would break ordinary
// scripts for no security gain.
func TestSyncTokenNeedsNoCSRFHeader(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintTokenWith(t, c, "nightly", true)
	c2 := &client{t: t, srv: c.srv}

	code, out := c2.withToken("POST", "/api/sync/runs", tok, map[string]string{
		"store": "appstore", "mode": "delta",
	})
	if errObj, _ := out["error"].(map[string]any); errObj["code"] == "csrf" {
		t.Fatalf("token request rejected for a missing CSRF header: %v", out)
	}
	if code == 403 {
		t.Fatalf("granted token refused: %v", out)
	}
}

// A capability is not a licence to write anything else.
func TestSyncTokenStillCannotWriteAnythingElse(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintTokenWith(t, c, "nightly", true)
	c2 := &client{t: t, srv: c.srv}

	writes := []struct{ method, path string }{
		{"POST", "/api/products"},
		{"PUT", "/api/settings"},
		{"DELETE", "/api/users/1"},
		{"PUT", "/api/me"},
		{"POST", "/api/me/tokens"},
	}
	for _, w := range writes {
		code, out := c2.withToken(w.method, w.path, tok, map[string]string{"name": "x"})
		if code != 403 {
			t.Fatalf("%s %s with sync token: want 403, got %d %v", w.method, w.path, code, out)
		}
	}
}

// A token must never exceed its owner: a viewer cannot run syncs, so a viewer's
// token cannot be granted the capability in the first place.
func TestViewerCannotCreateSyncToken(t *testing.T) {
	c, db := newTestServer(t, false)
	h, _ := auth.HashPassword("password123")
	if err := db.CreateUser(context.Background(), &models.User{
		Email: "viewer@x.io", Name: "V", Role: models.RoleViewer, PasswordHash: h,
	}); err != nil {
		t.Fatal(err)
	}
	if code, _ := c.do("POST", "/api/auth/login", map[string]string{
		"email": "viewer@x.io", "password": "password123",
	}); code != 200 {
		t.Fatal("viewer login failed")
	}

	code, out := c.do("POST", "/api/me/tokens", map[string]any{"name": "sneaky", "can_run_sync": true})
	if code == 201 {
		t.Fatalf("viewer was issued a sync-capable token: %v", out)
	}

	// A plain read-only token for the same viewer must still work.
	if code, _ := c.do("POST", "/api/me/tokens", map[string]any{"name": "fine", "can_run_sync": false}); code != 201 {
		t.Fatalf("viewer denied an ordinary read-only token: %d", code)
	}
}

// Capabilities are decided at creation. Existing tokens must not silently gain
// powers when the feature ships.
func TestExistingTokensDefaultToReadOnly(t *testing.T) {
	_, db := newTestServer(t, false)
	seedAdmin(t, db)
	ctx := context.Background()

	_, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	tok := &models.APIToken{UserID: 1, Name: "legacy", Hash: hash, Prefix: "oxi_abc"}
	if err := db.CreateAPIToken(ctx, tok); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetAPITokenByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.CanRunSync {
		t.Fatal("a token created without the capability came back with it")
	}
}
