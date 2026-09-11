// Package store is the SQLite repository. All SQL lives here.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps a single writer connection and a reader pool.
type DB struct {
	w *sql.DB
	r *sql.DB
}

func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"

	w, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	w.SetConnMaxLifetime(0)

	r, err := sql.Open("sqlite", dsn)
	if err != nil {
		w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(4)
	r.SetMaxIdleConns(4)
	r.SetConnMaxLifetime(0)

	if err := w.Ping(); err != nil {
		w.Close()
		r.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return &DB{w: w, r: r}, nil
}

func (d *DB) Close() error {
	rerr := d.r.Close()
	werr := d.w.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

// Writer returns the single-connection writer pool.
func (d *DB) Writer() *sql.DB { return d.w }

// Reader returns the read pool.
func (d *DB) Reader() *sql.DB { return d.r }

// Migrate applies all embedded migrations.
func (d *DB) Migrate(ctx context.Context) error {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	goose.SetBaseFS(sub)
	goose.SetLogger(gooseLogger{})
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.UpContext(ctx, d.w, ".")
}

// Ping is used by /api/health.
func (d *DB) Ping(ctx context.Context) error {
	var one int
	return d.r.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

// Tx runs fn in a write transaction.
func (d *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.w.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// BackupTo writes a consistent copy with VACUUM INTO.
func (d *DB) BackupTo(ctx context.Context, dest string) error {
	_, err := d.w.ExecContext(ctx, "VACUUM INTO ?", dest)
	return err
}

type gooseLogger struct{}

func (gooseLogger) Fatalf(format string, v ...any) { slog.Error(fmt.Sprintf(format, v...)) }
func (gooseLogger) Printf(format string, v ...any) { slog.Debug(fmt.Sprintf(format, v...)) }

// time helpers -------------------------------------------------------------

const tsLayout = time.RFC3339

func fmtTime(t time.Time) string { return t.UTC().Format(tsLayout) }

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := fmtTime(*t)
	return &s
}

func parseTime(s string) time.Time {
	t, err := time.Parse(tsLayout, s)
	if err != nil {
		// tolerate the space-separated form sqlite's datetime() emits
		t, _ = time.Parse("2006-01-02 15:04:05", s)
	}
	return t
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

func nullStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

func nullInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

func nullFloat(f sql.NullFloat64) *float64 {
	if !f.Valid {
		return nil
	}
	return &f.Float64
}

func now() time.Time { return time.Now().UTC() }
