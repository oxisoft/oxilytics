package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/oxisoft/oxilytics/internal/models"
)

const (
	// TokenPrefix makes a leaked token greppable in logs and recognisable in
	// a paste, the way GitHub's ghp_ prefix works.
	TokenPrefix = "oxi_"
	tokenBytes  = 32
	// PrefixLen is how much of the token is stored in clear for display.
	PrefixLen = len(TokenPrefix) + 6
)

type tokenCtxKey struct{}

// GenerateToken returns the plaintext token and its SHA-256 hash. Plaintext is
// returned to the caller exactly once and never persisted.
//
// SHA-256 rather than bcrypt: a token is 256 bits of CSPRNG output, so it has
// no guessable structure to slow an attacker down over, and it is verified on
// every API request where a deliberately slow hash would be a DoS vector.
func GenerateToken() (plaintext, hash string, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, HashToken(plaintext), nil
}

// HashToken is the one place the hashing scheme is defined, so storage and
// lookup cannot drift apart.
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// bearerToken extracts a token from the Authorization header. Only the Bearer
// scheme is accepted; query-string tokens are refused on purpose because they
// leak into access logs, browser history and Referer headers.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

// TokenMiddleware authenticates Bearer tokens and marks the request as
// token-authenticated. It runs before the session middleware and does not
// touch cookies, so the two schemes stay independent.
func (s *Service) TokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plaintext := bearerToken(r)
		if plaintext == "" {
			next.ServeHTTP(w, r)
			return
		}
		tok, err := s.db.GetAPITokenByHash(r.Context(), HashToken(plaintext))
		if err != nil {
			// An invalid token is an explicit failure, not a silent fallback
			// to anonymous: a caller with a revoked token must be told.
			writeTokenErr(w)
			return
		}
		u, err := s.db.GetUser(r.Context(), tok.UserID)
		if err != nil || u.Disabled {
			writeTokenErr(w)
			return
		}
		go func() {
			// Detached from the request context so a fast-returning response
			// does not cancel the write.
			_ = s.db.TouchAPIToken(context.Background(), tok.ID)
		}()
		ctx := context.WithValue(r.Context(), ctxKey{}, u)
		ctx = context.WithValue(ctx, tokenCtxKey{}, tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeTokenErr(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="oxilytics"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"invalid_token","message":"invalid or revoked API token"}}`))
}

// TokenFrom reports whether this request was authenticated by an API token.
// Handlers use it to refuse writes regardless of the user's role.
func TokenFrom(ctx context.Context) (*models.APIToken, bool) {
	t, ok := ctx.Value(tokenCtxKey{}).(*models.APIToken)
	return t, ok
}
