package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
)

func TestSchedulerAppliesSettingsAndBackup(t *testing.T) {
	f := &fakeIngester{st: models.StoreAppStore}
	e, db := newEngine(t, f)
	st := settings.New(db)
	dir := filepath.Join(t.TempDir(), "backups")
	s := NewScheduler(e, db, st, time.UTC, dir, 2)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if s.NextRun().IsZero() {
		t.Error("next run should be set (enabled by default)")
	}
	// change time → next run updates
	if _, err := st.Update(context.Background(), map[string]string{settings.KeyScheduleTime: "23:59"}); err != nil {
		t.Fatal(err)
	}
	if n := s.NextRun(); n.IsZero() || n.Hour() != 23 || n.Minute() != 59 {
		t.Errorf("next run = %v", n)
	}
	// disable → zero
	_, _ = st.Update(context.Background(), map[string]string{settings.KeyScheduleEnabled: "false"})
	if !s.NextRun().IsZero() {
		t.Error("should be disabled")
	}
	// backups rotate
	for i := 0; i < 3; i++ {
		if err := s.Backup(context.Background()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1100 * time.Millisecond)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("backups kept = %d, want 2", len(entries))
	}
}
