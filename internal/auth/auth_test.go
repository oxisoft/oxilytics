package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
)

func newSvc(t *testing.T) (*Service, *store.DB) {
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
	for i := range key {
		key[i] = byte(i)
	}
	return New(db, key, false, "test"), db
}

func seedUser(t *testing.T, db *store.DB, email, pw string) *models.User {
	h, _ := HashPassword(pw)
	u := &models.User{Email: email, Name: "U", PasswordHash: h, Role: models.RoleAdmin}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "s3cret") || CheckPassword(h, "nope") {
		t.Error("password check wrong")
	}
}

func TestLoginAndSession(t *testing.T) {
	svc, db := newSvc(t)
	u := seedUser(t, db, "a@b.c", "pw")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	if _, err := svc.Login(rec, req, "a@b.c", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong pw: %v", err)
	}
	if _, err := svc.Login(rec, req, "nobody@b.c", "pw"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown: %v", err)
	}
	rec = httptest.NewRecorder()
	got, err := svc.Login(rec, req, "A@B.C", "pw")
	if err != nil || got.ID != u.ID {
		t.Fatalf("login: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}

	// middleware loads the user from the cookie
	var seen *models.User
	h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = UserFromContext(r.Context()) }))
	r2 := httptest.NewRequest("GET", "/api/me", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}
	h.ServeHTTP(httptest.NewRecorder(), r2)
	if seen == nil || seen.ID != u.ID {
		t.Fatalf("middleware did not load user: %+v", seen)
	}

	// logout clears it
	rec3 := httptest.NewRecorder()
	svc.Logout(rec3, r2)
	r3 := httptest.NewRequest("GET", "/api/me", nil)
	for _, c := range rec3.Result().Cookies() {
		r3.AddCookie(c)
	}
	seen = nil
	h.ServeHTTP(httptest.NewRecorder(), r3)
	if seen != nil {
		t.Error("user still loaded after logout")
	}
}

func TestTOTPFlow(t *testing.T) {
	svc, db := newSvc(t)
	u := seedUser(t, db, "t@b.c", "pw")

	setup, err := svc.GenerateTOTP(u.Email)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, err := svc.EnableTOTP(context.Background(), u, setup.Secret, "000000"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad code accepted: %v", err)
	}
	recovery, err := svc.EnableTOTP(context.Background(), u, setup.Secret, code)
	if err != nil || len(recovery) != recoveryCount {
		t.Fatalf("enable: %v %d", err, len(recovery))
	}

	// login now needs a second step
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	if _, err := svc.Login(rec, req, u.Email, "pw"); !errors.Is(err, ErrTOTPRequired) {
		t.Fatalf("expected totp required, got %v", err)
	}
	r2 := httptest.NewRequest("POST", "/", nil)
	for _, c := range rec.Result().Cookies() {
		r2.AddCookie(c)
	}
	// wrong code
	if _, err := svc.LoginTOTP(httptest.NewRecorder(), r2, "123456"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong totp: %v", err)
	}
	// recovery code works once
	rec2 := httptest.NewRecorder()
	if _, err := svc.LoginTOTP(rec2, r2, recovery[0]); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	// establish a new pending session, same recovery code must fail now
	rec3 := httptest.NewRecorder()
	_, _ = svc.Login(rec3, httptest.NewRequest("POST", "/", nil), u.Email, "pw")
	r3 := httptest.NewRequest("POST", "/", nil)
	for _, c := range rec3.Result().Cookies() {
		r3.AddCookie(c)
	}
	if _, err := svc.LoginTOTP(httptest.NewRecorder(), r3, recovery[0]); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("recovery reused: %v", err)
	}
	// real code works
	code2, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, err := svc.LoginTOTP(httptest.NewRecorder(), r3, code2); err != nil {
		t.Fatalf("totp: %v", err)
	}

	// disable needs password
	fresh, _ := db.GetUser(context.Background(), u.ID)
	if err := svc.DisableTOTP(context.Background(), fresh, "bad"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("disable with bad pw")
	}
	if err := svc.DisableTOTP(context.Background(), fresh, "pw"); err != nil {
		t.Fatal(err)
	}
	fresh, _ = db.GetUser(context.Background(), u.ID)
	if fresh.TOTPEnabled {
		t.Error("still enabled")
	}
}

func TestRateLimit(t *testing.T) {
	svc, db := newSvc(t)
	seedUser(t, db, "r@b.c", "pw")
	req := httptest.NewRequest("POST", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	var last error
	for i := 0; i < 12; i++ {
		_, last = svc.Login(httptest.NewRecorder(), req, "r@b.c", "wrong")
	}
	if !errors.Is(last, ErrRateLimited) {
		t.Errorf("expected rate limit, got %v", last)
	}
}

func TestRequireFetchHeader(t *testing.T) {
	h := RequireFetchHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tt := range []struct {
		method, hdr string
		want        int
	}{
		{"GET", "", 204},
		{"POST", "", 403},
		{"POST", "fetch", 204},
		{"DELETE", "xhr", 403},
	} {
		req := httptest.NewRequest(tt.method, "/", nil)
		if tt.hdr != "" {
			req.Header.Set("X-Requested-With", tt.hdr)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s hdr=%q: %d want %d", tt.method, tt.hdr, rec.Code, tt.want)
		}
	}
}
