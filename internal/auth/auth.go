// Package auth: cookie sessions, passwords, TOTP, login rate limiting.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
)

const (
	sessionName    = "oxi_session"
	keyUserID      = "uid"
	keyPendingUser = "pending_uid" // set after password ok, before TOTP
	sessionMaxAge  = 30 * 24 * 60 * 60
	bcryptCost     = 12
	recoveryCount  = 8
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTOTPRequired       = errors.New("totp required")
	ErrRateLimited        = errors.New("rate limited")
	ErrDisabled           = errors.New("account disabled")
)

type Service struct {
	db      *store.DB
	store   *sessions.CookieStore
	limiter *rateLimiter
	issuer  string
}

func New(db *store.DB, key []byte, secure bool, issuer string) *Service {
	cs := sessions.NewCookieStore(key[:32], key[:32])
	cs.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	return &Service{db: db, store: cs, limiter: newRateLimiter(10, time.Minute), issuer: issuer}
}

// passwords ------------------------------------------------------------------

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// sessions -------------------------------------------------------------------

type ctxKey struct{}

// UserFromContext returns the authenticated user set by Middleware.
func UserFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(ctxKey{}).(*models.User)
	return u
}

// Middleware loads the session user (if any) into the request context.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A token-authenticated request must not be re-attributed to whatever
		// cookie happens to be attached, or a stale browser session could
		// silently upgrade a token call.
		if _, viaToken := TokenFrom(r.Context()); viaToken {
			next.ServeHTTP(w, r)
			return
		}
		sess, _ := s.store.Get(r, sessionName)
		if id, ok := sess.Values[keyUserID].(int64); ok {
			u, err := s.db.GetUser(r.Context(), id)
			if err == nil && !u.Disabled {
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Login step 1. Returns ErrTOTPRequired when the user has TOTP enabled; the
// session then carries a pending id that LoginTOTP completes.
func (s *Service) Login(w http.ResponseWriter, r *http.Request, email, password string) (*models.User, error) {
	ip := clientIP(r)
	if !s.limiter.allow(ip) {
		return nil, ErrRateLimited
	}
	u, err := s.db.GetUserByEmail(r.Context(), email)
	if err != nil || !CheckPassword(u.PasswordHash, password) {
		// constant-ish time: still run bcrypt on a dummy when the user is unknown
		if err != nil {
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$000000000000000000000uGZLxlSEWq4t8Uj1IuF8xgSjPfxjQ4Oy"), []byte(password))
		}
		return nil, ErrInvalidCredentials
	}
	if u.Disabled {
		return nil, ErrDisabled
	}
	sess, _ := s.store.Get(r, sessionName)
	if u.TOTPEnabled {
		sess.Values = map[any]any{keyPendingUser: u.ID}
		if err := sess.Save(r, w); err != nil {
			return nil, err
		}
		return u, ErrTOTPRequired
	}
	return u, s.establish(w, r, sess, u)
}

// LoginTOTP completes a pending login with a TOTP code or a recovery code.
func (s *Service) LoginTOTP(w http.ResponseWriter, r *http.Request, code string) (*models.User, error) {
	if !s.limiter.allow(clientIP(r)) {
		return nil, ErrRateLimited
	}
	sess, _ := s.store.Get(r, sessionName)
	id, ok := sess.Values[keyPendingUser].(int64)
	if !ok {
		return nil, ErrInvalidCredentials
	}
	u, err := s.db.GetUser(r.Context(), id)
	if err != nil || u.Disabled || !u.TOTPEnabled {
		return nil, ErrInvalidCredentials
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if totp.Validate(code, *u.TOTPSecret) {
		return u, s.establish(w, r, sess, u)
	}
	if s.consumeRecovery(r.Context(), u, code) {
		return u, s.establish(w, r, sess, u)
	}
	return nil, ErrInvalidCredentials
}

func (s *Service) establish(w http.ResponseWriter, r *http.Request, sess *sessions.Session, u *models.User) error {
	sess.Values = map[any]any{keyUserID: u.ID}
	if err := sess.Save(r, w); err != nil {
		return err
	}
	return s.db.TouchUserLogin(r.Context(), u.ID)
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.store.Get(r, sessionName)
	sess.Options.MaxAge = -1
	sess.Values = map[any]any{}
	_ = sess.Save(r, w)
}

// TOTP -----------------------------------------------------------------------

type TOTPSetup struct {
	Secret string `json:"secret"`
	URL    string `json:"otpauth_url"`
}

// GenerateTOTP returns a fresh secret; nothing is stored until EnableTOTP.
func (s *Service) GenerateTOTP(email string) (*TOTPSetup, error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.issuer, AccountName: email, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return nil, err
	}
	return &TOTPSetup{Secret: key.Secret(), URL: key.URL()}, nil
}

// EnableTOTP verifies the first code against the secret, stores it and returns
// the plaintext recovery codes (shown once).
func (s *Service) EnableTOTP(ctx context.Context, u *models.User, secret, code string) ([]string, error) {
	if !totp.Validate(strings.ReplaceAll(code, " ", ""), secret) {
		return nil, ErrInvalidCredentials
	}
	codes := make([]string, recoveryCount)
	hashes := make([]string, recoveryCount)
	for i := range codes {
		c, err := randomCode()
		if err != nil {
			return nil, err
		}
		h, err := bcrypt.GenerateFromPassword([]byte(c), bcrypt.MinCost+2)
		if err != nil {
			return nil, err
		}
		codes[i], hashes[i] = c, string(h)
	}
	js, _ := json.Marshal(hashes)
	rec := string(js)
	if err := s.db.UpdateUserTOTP(ctx, u.ID, &secret, &rec); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) DisableTOTP(ctx context.Context, u *models.User, password string) error {
	if !CheckPassword(u.PasswordHash, password) {
		return ErrInvalidCredentials
	}
	return s.db.UpdateUserTOTP(ctx, u.ID, nil, nil)
}

func (s *Service) consumeRecovery(ctx context.Context, u *models.User, code string) bool {
	if u.TOTPRecovery == nil {
		return false
	}
	var hashes []string
	if err := json.Unmarshal([]byte(*u.TOTPRecovery), &hashes); err != nil {
		return false
	}
	for i, h := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(code)) == nil {
			hashes = append(hashes[:i], hashes[i+1:]...)
			js, _ := json.Marshal(hashes)
			rec := string(js)
			_ = s.db.UpdateUserTOTP(ctx, u.ID, u.TOTPSecret, &rec)
			return true
		}
	}
	return false
}

func randomCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	return s[:5] + "-" + s[5:10] + "-" + s[10:15], nil
}

// CSRF -----------------------------------------------------------------------

// RequireFetchHeader rejects mutating requests without X-Requested-With: fetch.
func RequireFetchHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Requested-With")), []byte("fetch")) != 1 {
				http.Error(w, `{"error":{"code":"csrf","message":"missing X-Requested-With header"}}`, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rate limiter ---------------------------------------------------------------

type rateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-l.window)
	h := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			h = append(h, t)
		}
	}
	if len(h) >= l.max {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	return true
}

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}
