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
			r.Post("/auth/logout", a.logout)
			r.Get("/me", a.me)
			r.Put("/me", a.updateMe)
			r.Put("/me/password", a.changePassword)
			r.Post("/me/totp/setup", a.totpSetup)
			r.Post("/me/totp/enable", a.totpEnable)
			r.Delete("/me/totp", a.totpDisable)

			r.Get("/setup/status", a.setupStatus)
			r.Get("/setup/guide/{store}", a.setupGuide)
			r.With(a.require(permissions.TestStore)).Post("/setup/test/{store}", a.setupTest)

			r.Get("/settings", a.getSettings)
			r.With(a.require(permissions.EditSettings)).Put("/settings", a.putSettings)

			r.Route("/users", func(r chi.Router) {
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
