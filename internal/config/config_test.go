package config

import (
	"log/slog"
	"testing"
)

func TestLoad(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
		check   func(t *testing.T, c *Config)
	}{
		{
			name:    "missing session key",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name:    "short session key",
			env:     map[string]string{"OXI_SESSION_KEY": "abcd"},
			wantErr: true,
		},
		{
			name: "defaults",
			env:  map[string]string{"OXI_SESSION_KEY": key},
			check: func(t *testing.T, c *Config) {
				if c.Addr != "127.0.0.1:8080" {
					t.Errorf("Addr = %q", c.Addr)
				}
				if c.Secure {
					t.Error("Secure should be false for http base url")
				}
				if c.TZ.String() != "UTC" {
					t.Errorf("TZ = %s", c.TZ)
				}
				if c.LogLevel != slog.LevelInfo {
					t.Errorf("LogLevel = %v", c.LogLevel)
				}
				if c.BackupKeep != 14 {
					t.Errorf("BackupKeep = %d", c.BackupKeep)
				}
			},
		},
		{
			name: "https base url sets secure",
			env:  map[string]string{"OXI_SESSION_KEY": key, "OXI_BASE_URL": "https://a.example.com/"},
			check: func(t *testing.T, c *Config) {
				if !c.Secure {
					t.Error("Secure should be true")
				}
				if c.BaseURL != "https://a.example.com" {
					t.Errorf("BaseURL not trimmed: %q", c.BaseURL)
				}
			},
		},
		{
			name:    "bad tz",
			env:     map[string]string{"OXI_SESSION_KEY": key, "OXI_TZ": "Mars/Olympus"},
			wantErr: true,
		},
		{
			name:    "bad log level",
			env:     map[string]string{"OXI_SESSION_KEY": key, "OXI_LOG_LEVEL": "loud"},
			wantErr: true,
		},
		{
			name:    "bootstrap email without password",
			env:     map[string]string{"OXI_SESSION_KEY": key, "OXI_BOOTSTRAP_ADMIN_EMAIL": "a@b.c"},
			wantErr: true,
		},
		{
			name: "bootstrap email lowercased",
			env:  map[string]string{"OXI_SESSION_KEY": key, "OXI_BOOTSTRAP_ADMIN_EMAIL": " Admin@Example.COM ", "OXI_BOOTSTRAP_ADMIN_PASSWORD": "x"},
			check: func(t *testing.T, c *Config) {
				if c.BootstrapAdminEmail != "admin@example.com" {
					t.Errorf("email = %q", c.BootstrapAdminEmail)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"OXI_SESSION_KEY", "OXI_BASE_URL", "OXI_TZ", "OXI_LOG_LEVEL", "OXI_BOOTSTRAP_ADMIN_EMAIL", "OXI_BOOTSTRAP_ADMIN_PASSWORD", "OXI_BACKUP_KEEP", "OXI_ADDR"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			c, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}
