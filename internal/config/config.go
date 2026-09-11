// Package config reads the OXI_* environment variables into a validated Config.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr     string
	DBPath   string
	BaseURL  string
	Secure   bool // cookie Secure flag, derived from BaseURL
	TZ       *time.Location
	LogLevel slog.Level

	SessionKey []byte // 32 bytes, decoded from 64 hex chars

	BootstrapAdminEmail    string
	BootstrapAdminPassword string

	ASC   ASCConfig
	GPlay GPlayConfig

	BackupDir  string
	BackupKeep int
}

type ASCConfig struct {
	KeyID    string
	IssuerID string
	KeyFile  string
}

type GPlayConfig struct {
	SAFile string
	Bucket string
}

// Load reads the environment. It returns an error for anything that would make
// the process misbehave; store credentials are NOT validated here (see setup).
func Load() (*Config, error) {
	c := &Config{
		Addr:       env("OXI_ADDR", "127.0.0.1:8080"),
		DBPath:     env("OXI_DB_PATH", "/data/oxilytics.db"),
		BaseURL:    strings.TrimRight(env("OXI_BASE_URL", "http://localhost:8080"), "/"),
		BackupDir:  env("OXI_BACKUP_DIR", "/data/backups"),
		BackupKeep: 14,
		ASC: ASCConfig{
			KeyID:    os.Getenv("OXI_ASC_KEY_ID"),
			IssuerID: os.Getenv("OXI_ASC_ISSUER_ID"),
			KeyFile:  env("OXI_ASC_KEY_FILE", "/secrets/AuthKey.p8"),
		},
		GPlay: GPlayConfig{
			SAFile: env("OXI_GPLAY_SA_FILE", "/secrets/gplay-sa.json"),
			Bucket: os.Getenv("OXI_GPLAY_BUCKET"),
		},
		BootstrapAdminEmail:    strings.ToLower(strings.TrimSpace(os.Getenv("OXI_BOOTSTRAP_ADMIN_EMAIL"))),
		BootstrapAdminPassword: os.Getenv("OXI_BOOTSTRAP_ADMIN_PASSWORD"),
	}

	var errs []error

	keyHex := os.Getenv("OXI_SESSION_KEY")
	if keyHex == "" {
		errs = append(errs, errors.New("OXI_SESSION_KEY is required (64 hex chars; generate with: openssl rand -hex 32)"))
	} else {
		key, err := hex.DecodeString(keyHex)
		if err != nil || len(key) != 32 {
			errs = append(errs, errors.New("OXI_SESSION_KEY must be exactly 64 hex characters"))
		}
		c.SessionKey = key
	}

	c.Secure = strings.HasPrefix(c.BaseURL, "https://")

	tzName := env("OXI_TZ", "UTC")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		errs = append(errs, fmt.Errorf("OXI_TZ %q: %w", tzName, err))
	}
	c.TZ = loc

	switch strings.ToLower(env("OXI_LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn", "warning":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		errs = append(errs, fmt.Errorf("OXI_LOG_LEVEL %q: want debug|info|warn|error", os.Getenv("OXI_LOG_LEVEL")))
	}

	if v := os.Getenv("OXI_BACKUP_KEEP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			errs = append(errs, fmt.Errorf("OXI_BACKUP_KEEP %q: want a non-negative integer", v))
		}
		c.BackupKeep = n
	}

	if (c.BootstrapAdminEmail == "") != (c.BootstrapAdminPassword == "") {
		errs = append(errs, errors.New("OXI_BOOTSTRAP_ADMIN_EMAIL and OXI_BOOTSTRAP_ADMIN_PASSWORD must be set together"))
	}

	return c, errors.Join(errs...)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
