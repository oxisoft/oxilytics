package sync

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/store"
)

type fakeIngester struct {
	st      models.Store
	apps    []models.App
	perApp  func(ctx context.Context, rc *RunContext, app models.App) error
	started int32
}

func (f *fakeIngester) Store() models.Store { return f.st }
func (f *fakeIngester) DiscoverApps(ctx context.Context, rc *RunContext) ([]models.App, error) {
	atomic.AddInt32(&f.started, 1)
	return f.apps, nil
}
func (f *fakeIngester) SyncApp(ctx context.Context, rc *RunContext, app models.App) error {
	return f.perApp(ctx, rc, app)
}

func newEngine(t *testing.T, ing ...Ingester) (*Engine, *store.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewEngine(db, settings.New(db), time.UTC, ing...), db
}

func seedApps(t *testing.T, db *store.DB, st models.Store, n int) []models.App {
	var out []models.App
	for i := 0; i < n; i++ {
		a, _, err := db.UpsertApp(context.Background(), &models.App{Store: st, StoreAppID: string(rune('a' + i)), Name: "App", Platform: models.PlatformIOS})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, *a)
	}
	return out
}

func TestRunSucceedsAndRecordsProgress(t *testing.T) {
	var db *store.DB
	f := &fakeIngester{st: models.StoreAppStore}
	e, db := newEngine(t, f)
	f.apps = seedApps(t, db, models.StoreAppStore, 3)
	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		rc.AddRows(10, 2)
		rc.Log("info", &app.ID, "ok")
		return nil
	}
	run, err := e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	if err != nil {
		t.Fatal(err)
	}
	// second start while running → conflict
	if _, err := e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("expected already running, got %v", err)
	}
	e.Wait(models.StoreAppStore)
	got, _ := db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncSucceeded || got.AppsDone != 3 || got.RowsMetrics != 30 || got.RowsReviews != 6 || got.Stats == nil {
		t.Errorf("run: %+v", got)
	}
	logs, _ := db.ListSyncLogs(context.Background(), run.ID, 0, 100)
	if len(logs) < 5 {
		t.Errorf("logs = %d", len(logs))
	}
	if e.Running(models.StoreAppStore) != nil {
		t.Error("still marked running")
	}
	// unconfigured store
	if _, err := e.Start(context.Background(), models.StoreGooglePlay, models.SyncDelta, models.TriggerManual, nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected not configured, got %v", err)
	}
}

func TestRunPartialFailureAndFatal(t *testing.T) {
	f := &fakeIngester{st: models.StoreAppStore}
	e, db := newEngine(t, f)
	f.apps = seedApps(t, db, models.StoreAppStore, 3)
	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		if app.StoreAppID == "b" {
			return errors.New("boom")
		}
		return nil
	}
	run, _ := e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	e.Wait(models.StoreAppStore)
	got, _ := db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncSucceeded || got.AppsDone != 3 {
		t.Errorf("partial failure should still succeed: %+v", got)
	}

	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error { return errors.New("all bad") }
	run, _ = e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	e.Wait(models.StoreAppStore)
	got, _ = db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncFailed {
		t.Errorf("all failed → failed: %+v", got)
	}

	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		return &FatalError{Err: errors.New("401")}
	}
	run, _ = e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	e.Wait(models.StoreAppStore)
	got, _ = db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncFailed || got.AppsDone != 0 {
		t.Errorf("fatal should abort immediately: %+v", got)
	}
}

func TestCancel(t *testing.T) {
	f := &fakeIngester{st: models.StoreGooglePlay}
	e, db := newEngine(t, f)
	f.apps = seedApps(t, db, models.StoreGooglePlay, 5)
	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		select {
		case <-time.After(200 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	run, _ := e.Start(context.Background(), models.StoreGooglePlay, models.SyncFull, models.TriggerManual, nil)
	time.Sleep(50 * time.Millisecond)
	if err := e.Cancel(models.StoreGooglePlay); err != nil {
		t.Fatal(err)
	}
	e.Wait(models.StoreGooglePlay)
	got, _ := db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncCancelled || got.AppsDone >= 5 {
		t.Errorf("cancel: %+v", got)
	}
	if err := e.Cancel(models.StoreGooglePlay); !errors.Is(err, ErrNotRunning) {
		t.Errorf("cancel idle: %v", err)
	}
}

func TestDeltaFrom(t *testing.T) {
	rc := &RunContext{Mode: models.SyncDelta, From: "2026-09-08", OverlapDays: 3}
	if got := DeltaFrom(rc, "2026-09-10"); got != "2026-09-07" {
		t.Errorf("delta from checkpoint = %s", got)
	}
	if got := DeltaFrom(rc, ""); got != "2008-07-10" {
		t.Errorf("delta no checkpoint = %s", got)
	}
	rc.Mode = models.SyncFull
	if got := DeltaFrom(rc, "2026-09-10"); got != rc.From {
		t.Errorf("full = %s", got)
	}
}

func TestSuggestProductNeverLinks(t *testing.T) {
	_, db := newEngine(t)
	ctx := context.Background()
	p := &models.Product{Name: "My Notes"}
	_ = db.CreateProduct(ctx, p)
	app, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: "1", Name: "my notes", Platform: models.PlatformIOS})
	rc := &RunContext{DB: db, Suggest: true, Log: func(string, *int64, string, ...any) {}}
	SuggestProduct(ctx, rc, app)
	got, _ := db.GetApp(ctx, app.ID)
	if got.ProductID != nil {
		t.Error("must not auto-link")
	}
	if got.SuggestedProductID == nil || *got.SuggestedProductID != p.ID {
		t.Errorf("suggestion missing: %+v", got)
	}
	// suggestions off
	app2, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreGooglePlay, StoreAppID: "2", Name: "My Notes", Platform: models.PlatformAndroid})
	rc.Suggest = false
	SuggestProduct(ctx, rc, app2)
	got, _ = db.GetApp(ctx, app2.ID)
	if got.SuggestedProductID != nil {
		t.Error("suggested while disabled")
	}
}
