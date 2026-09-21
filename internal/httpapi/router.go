package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/config"
	"github.com/oxisoft/oxilytics/internal/permissions"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/setup"
	"github.com/oxisoft/oxilytics/internal/store"
)

// Deps are the services the API is wired to. Optional ones may be nil while a
// milestone is not implemented; their routes then answer 501.
type Deps struct {
	Cfg      *config.Config
	DB       *store.DB
	Auth     *auth.Service
	Settings *settings.Service
	Setup    setup.Status
	Tester   StoreTester // live connection test; nil → 501
	Sync     SyncAPI     // nil → 501
	WebFS    fs.FS       // embedded SPA; nil → 404 for non-API paths
}

type API struct {
	Deps
}

func NewRouter(d Deps) http.Handler {
	a := &API{Deps: d}
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(d.Auth.TokenMiddleware)
	r.Use(d.Auth.Middleware)
	r.Use(requestLogger)

	r.Route("/api", func(r chi.Router) {
		r.Use(auth.RequireFetchHeader)
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) { notFound(w) })

		// public
		r.Get("/health", a.health)
		r.Get("/version", a.version)
		r.Post("/auth/login", a.login)
		r.Post("/auth/totp", a.loginTOTP)

		// authenticated
		r.Group(func(r chi.Router) {
			r.Use(a.requireUser)
			r.Use(a.readOnlyForTokens)
			r.Post("/auth/logout", a.logout)
			r.Get("/me", a.me)
			r.Put("/me", a.updateMe)
			r.Put("/me/password", a.changePassword)
			r.Post("/me/totp/setup", a.totpSetup)
			r.Post("/me/totp/enable", a.totpEnable)
			r.Delete("/me/totp", a.totpDisable)

			// Token management is session-only: a token must never be able to
			// mint or revoke tokens, including its own.
			r.Group(func(r chi.Router) {
				r.Use(a.requireSession)
				a.mountTokens(r)
			})

			r.Get("/setup/status", a.setupStatus)
			r.Get("/setup/guide/{store}", a.setupGuide)
			r.With(a.require(permissions.TestStore)).Post("/setup/test/{store}", a.setupTest)

			r.Get("/settings", a.getSettings)
			r.With(a.require(permissions.EditSettings)).Put("/settings", a.putSettings)

			r.Route("/users", func(r chi.Router) {
				r.Use(a.requireSession)
				r.Use(a.require(permissions.ManageUsers))
				r.Get("/", a.listUsers)
				r.Post("/", a.createUser)
				r.Put("/{id}", a.updateUser)
				r.Put("/{id}/password", a.resetPassword)
				r.Delete("/{id}", a.deleteUser)
			})

			// everything below is gated by setup mode
			r.Group(func(r chi.Router) {
				r.Use(a.requireConfigured)
				a.mountData(r)
			})
		})
	})

	r.Get("/*", a.spa)
	return r
}

// middleware -----------------------------------------------------------------

func (a *API) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.UserFromContext(r.Context()) == nil {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) require(action permissions.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !permissions.Can(auth.UserFromContext(r.Context()), action) {
				forbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireSession rejects token-authenticated callers from routes that only a
// logged-in human should reach.
func (a *API) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, viaToken := auth.TokenFrom(r.Context()); viaToken {
			writeErr(w, http.StatusForbidden, "session_required",
				"this endpoint requires an interactive sign-in, not an API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// readOnlyForTokens rejects any non-read request made with an API token,
// except the narrow set of capabilities explicitly granted to that token.
//
// Enforced here as a blanket method check rather than per handler: a new write
// endpoint added later is denied by default, which is the opposite of the
// usual failure mode where someone forgets to annotate a route. The method
// whitelist is the guarantee — tokens can never write, whatever the owner's
// role is or becomes.
//
// Capabilities are an allowlist of exact method+path pairs, not a role. A
// capability must name precisely one endpoint, so granting one can never widen
// into another: "can run a sync" means POST /api/sync/runs and nothing else.
// Matching is on the routing pattern rather than the raw URL so that path
// parameters cannot be used to slip past it.
func (a *API) readOnlyForTokens(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, viaToken := auth.TokenFrom(r.Context())
		if !viaToken {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if tok.CanRunSync && isSyncStart(r) {
			// The route itself still applies permissions.RunSync, so a token
			// cannot exceed what its owner is allowed to do.
			next.ServeHTTP(w, r)
			return
		}
		writeErr(w, http.StatusForbidden, "read_only_token",
			"API tokens are read-only; use the web interface to make changes")
	})
}

// isSyncStart matches exactly POST /api/sync/runs — the collection endpoint
// that starts a run. It must not match POST /api/sync/runs/{id}/cancel or
// /api/sync/reset, which are separate powers nobody granted.
//
// The raw path is used deliberately. chi's RoutePattern() is only resolved
// once routing reaches the final handler; inside a group middleware it still
// returns the partial pattern (/api/*), which silently matches nothing. The
// path is normalised with path.Clean first so that /api/sync/runs/ and
// /api/sync/runs/../runs cannot produce a different answer here than the
// router will reach.
func isSyncStart(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	return path.Clean(r.URL.Path) == "/api/sync/runs"
}

func (a *API) requireConfigured(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Setup.SetupRequired {
			writeErr(w, http.StatusServiceUnavailable, "setup_required", "configure at least one store first")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			var uid int64
			if u := auth.UserFromContext(r.Context()); u != nil {
				uid = u.ID
			}
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "ms", time.Since(start).Milliseconds(), "user", uid)
		}
	})
}

// SPA ------------------------------------------------------------------------

func (a *API) spa(w http.ResponseWriter, r *http.Request) {
	if a.WebFS == nil {
		http.Error(w, "frontend not built", http.StatusNotFound)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := a.WebFS.Open(p); err == nil {
		if st, err := f.Stat(); err == nil && !st.IsDir() {
			f.Close()
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, a.WebFS, p)
			return
		}
		f.Close()
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, a.WebFS, "index.html")
}
