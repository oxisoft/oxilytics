// Command server is the Oxilytics binary: config → DB → migrations → services → HTTP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/config"
	"github.com/oxisoft/oxilytics/internal/httpapi"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/setup"
	"github.com/oxisoft/oxilytics/internal/store"
	"github.com/oxisoft/oxilytics/internal/tester"
	"github.com/oxisoft/oxilytics/internal/version"
	"github.com/oxisoft/oxilytics/web"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the running server and exit (for Docker HEALTHCHECK)")
	flag.Parse()

	if *healthcheck {
		os.Exit(runHealthcheck())
	}
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))
	slog.Info("starting oxilytics", "version", version.Version, "commit", version.Commit, "addr", cfg.Addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := bootstrapAdmin(ctx, db, cfg); err != nil {
		return err
	}

	st := setup.Evaluate(cfg)
	for s, ss := range st.Stores {
		slog.Info("store", "store", s, "configured", ss.Configured)
	}
	if st.SetupRequired {
		slog.Warn("no store configured — running in setup mode; open the UI and follow the guide")
	}

	authSvc := auth.New(db, cfg.SessionKey, cfg.Secure, "Oxilytics")
	settingsSvc := settings.New(db)

	webFS, err := web.FS()
	if err != nil {
		return fmt.Errorf("web fs: %w", err)
	}

	handler := httpapi.NewRouter(httpapi.Deps{
		Cfg:      cfg,
		DB:       db,
		Auth:     authSvc,
		Settings: settingsSvc,
		Setup:    st,
		Tester:   tester.New(cfg),
		WebFS:    webFS,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func bootstrapAdmin(ctx context.Context, db *store.DB, cfg *config.Config) error {
	n, err := db.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.BootstrapAdminEmail == "" {
		slog.Warn("no users exist and OXI_BOOTSTRAP_ADMIN_EMAIL/PASSWORD are not set — nobody can log in")
		return nil
	}
	hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	u := &models.User{Email: cfg.BootstrapAdminEmail, Name: "Admin", Role: models.RoleAdmin, PasswordHash: hash}
	if err := db.CreateUser(ctx, u); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	slog.Info("bootstrap admin created", "email", u.Email)
	return nil
}

func runHealthcheck() int {
	addr := os.Getenv("OXI_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if host, port, err := net.SplitHostPort(addr); err == nil && (host == "" || host == "0.0.0.0") {
		addr = net.JoinHostPort("127.0.0.1", port)
	}
	c := &http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get("http://" + addr + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Fprintln(os.Stderr, "status", resp.StatusCode)
		return 1
	}
	return 0
}
