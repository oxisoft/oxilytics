package store

import (
	"context"
	"path/filepath"
	"testing"
)

// testDB opens a fresh temporary SQLite file with all migrations applied.
func testDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func TestMigrateIsIdempotent(t *testing.T) {
	d := testDB(t)
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	s, err := d.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s["sync.schedule.time"] != "06:30" {
		t.Errorf("default setting missing: %v", s)
	}
}

func TestPragmas(t *testing.T) {
	d := testDB(t)
	var mode string
	if err := d.Reader().QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
	var fk int
	if err := d.Writer().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}
