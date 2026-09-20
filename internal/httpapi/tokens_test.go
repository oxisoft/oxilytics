package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

// tokenClient issues requests with a Bearer token and no cookies at all, which
// is how a real script calls the API.
func (c *client) withToken(method, path, token string, body any) (int, map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Requested-With", "fetch")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func login(t *testing.T, c *client) {
	t.Helper()
	if code, _ := c.do("POST", "/api/auth/login", map[string]string{"email": "admin@x.io", "password": "password123"}); code != 200 {
		t.Fatalf("login failed: %d", code)
	}
}

// mintToken creates a token through the API and returns its plaintext.
func mintToken(t *testing.T, c *client, name string) string {
	t.Helper()
	code, out := c.do("POST", "/api/me/tokens", map[string]string{"name": name})
	if code != 201 {
		t.Fatalf("create token: %d %v", code, out)
	}
	tok, _ := out["token"].(string)
	if tok == "" {
		t.Fatal("no plaintext token in create response")
	}
	return tok
}

func TestAPITokenReadAccess(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintToken(t, c, "reporting")

	// A token with no cookie must be able to read.
	c2, _ := &client{t: t, srv: c.srv}, 0
	if code, out := c2.withToken("GET", "/api/me", tok, nil); code != 200 || out["email"] != "admin@x.io" {
		t.Fatalf("token read /api/me: %d %v", code, out)
	}
	if code, _ := c2.withToken("GET", "/api/products", tok, nil); code != 200 {
		t.Fatalf("token read /api/products: %d", code)
	}
}

// The core guarantee: a token belonging to an admin still cannot write.
func TestAPITokenIsReadOnly(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintToken(t, c, "reporting")
	c2 := &client{t: t, srv: c.srv}

	writes := []struct{ method, path string }{
		{"POST", "/api/products"},
		{"PUT", "/api/settings"},
		{"DELETE", "/api/users/1"},
		{"PUT", "/api/me"},
		{"POST", "/api/sync/run"},
	}
	for _, w := range writes {
		code, out := c2.withToken(w.method, w.path, tok, map[string]string{"name": "x"})
		if code != http.StatusForbidden {
			t.Fatalf("%s %s with token: got %d, want 403 (body %v)", w.method, w.path, code, out)
		}
	}
}

// A token must not be able to mint or revoke tokens, or it could escalate its
// own lifetime past a revocation.
func TestAPITokenCannotManageTokens(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintToken(t, c, "reporting")
	c2 := &client{t: t, srv: c.srv}

	if code, _ := c2.withToken("GET", "/api/me/tokens", tok, nil); code != http.StatusForbidden {
		t.Fatalf("token listing tokens: got %d, want 403", code)
	}
	if code, _ := c2.withToken("POST", "/api/me/tokens", tok, map[string]string{"name": "second"}); code != http.StatusForbidden {
		t.Fatalf("token minting token: got %d, want 403", code)
	}
}

func TestAPITokenRevocation(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintToken(t, c, "reporting")
	c2 := &client{t: t, srv: c.srv}

	if code, _ := c2.withToken("GET", "/api/me", tok, nil); code != 200 {
		t.Fatalf("token should work before revoke: %d", code)
	}

	var list []map[string]any
	if code := c.doRaw("GET", "/api/me/tokens", &list); code != 200 || len(list) != 1 {
		t.Fatalf("list tokens: %d %v", code, list)
	}
	id := int64(list[0]["id"].(float64))

	// The plaintext must never be readable after creation.
	if s, ok := list[0]["token"].(string); ok && s != "" {
		t.Fatal("plaintext token leaked in the list response")
	}

	if code, _ := c.do("DELETE", "/api/me/tokens/"+itoa(id), nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, out := c2.withToken("GET", "/api/me", tok, nil); code != 401 {
		t.Fatalf("revoked token still works: %d %v", code, out)
	}
}

func TestAPITokenRejectsGarbage(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	c2 := &client{t: t, srv: c.srv}
	for _, bad := range []string{"oxi_nonsense", "", "Bearer", "abc123"} {
		if bad == "" {
			continue
		}
		if code, _ := c2.withToken("GET", "/api/me", bad, nil); code != 401 {
			t.Fatalf("garbage token %q: got %d, want 401", bad, code)
		}
	}
}

// A disabled owner's tokens must stop working immediately.
func TestAPITokenDiesWithDisabledUser(t *testing.T) {
	c, db := newTestServer(t, false)
	seedAdmin(t, db)
	login(t, c)
	tok := mintToken(t, c, "reporting")
	c2 := &client{t: t, srv: c.srv}

	u, err := db.GetUserByEmail(t.Context(), "admin@x.io")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateUser(t.Context(), u.ID, u.Name, u.Role, true); err != nil {
		t.Fatal(err)
	}
	if code, _ := c2.withToken("GET", "/api/me", tok, nil); code != 401 {
		t.Fatalf("token of disabled user: got %d, want 401", code)
	}
}
