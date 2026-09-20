package sync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
)

// concurrentFake opts into parallel app syncing.
type concurrentFake struct {
	fakeIngester
	workers int
}

func (f *concurrentFake) AppConcurrency() int { return f.workers }

// A slow app must not block the others. Sequentially this takes 5 × 200ms;
// with 5 workers it should finish in roughly one slot.
func TestConcurrentAppsRunInParallel(t *testing.T) {
	f := &concurrentFake{fakeIngester: fakeIngester{st: models.StoreAppStore}, workers: 5}
	e, db := newEngine(t, f)
	f.apps = seedApps(t, db, models.StoreAppStore, 5)

	var inFlight, peak int32
	var mu sync.Mutex
	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		n := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(200 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	}

	start := time.Now()
	run, _ := e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	e.Wait(models.StoreAppStore)
	elapsed := time.Since(start)

	got, _ := db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncSucceeded || got.AppsDone != 5 {
		t.Fatalf("run: %+v", got)
	}
	mu.Lock()
	p := peak
	mu.Unlock()
	if p < 2 {
		t.Fatalf("apps did not run concurrently: peak in-flight %d", p)
	}
	if elapsed > 900*time.Millisecond {
		t.Fatalf("took %s; expected roughly one 200ms slot, not 5 sequential ones", elapsed)
	}
}

// Concurrency must be bounded, or a large catalogue would hammer the store API.
func TestConcurrencyIsBounded(t *testing.T) {
	f := &concurrentFake{fakeIngester: fakeIngester{st: models.StoreAppStore}, workers: 2}
	e, db := newEngine(t, f)
	f.apps = seedApps(t, db, models.StoreAppStore, 8)

	var inFlight, peak int32
	var mu sync.Mutex
	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		n := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	}
	e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	e.Wait(models.StoreAppStore)

	mu.Lock()
	p := peak
	mu.Unlock()
	if p > 2 {
		t.Fatalf("peak in-flight %d exceeds the configured limit of 2", p)
	}
}

// A fatal error must stop the run promptly rather than letting every remaining
// app repeat the same failure.
func TestConcurrentFatalCancelsRemaining(t *testing.T) {
	f := &concurrentFake{fakeIngester: fakeIngester{st: models.StoreAppStore}, workers: 2}
	e, db := newEngine(t, f)
	f.apps = seedApps(t, db, models.StoreAppStore, 10)

	var attempts int32
	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		atomic.AddInt32(&attempts, 1)
		return &FatalError{Err: errors.New("401")}
	}
	run, _ := e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	e.Wait(models.StoreAppStore)

	got, _ := db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncFailed {
		t.Fatalf("expected failed, got %+v", got)
	}
	// With 2 workers a couple of apps may already be in flight when the first
	// fatal lands, but the run must not plough through all ten.
	if n := atomic.LoadInt32(&attempts); n > 4 {
		t.Fatalf("fatal did not cancel remaining apps: %d of 10 attempted", n)
	}
}

// Failures from several apps at once must all be recorded, not lost to a race.
func TestConcurrentErrorsAreAllCollected(t *testing.T) {
	f := &concurrentFake{fakeIngester: fakeIngester{st: models.StoreAppStore}, workers: 4}
	e, db := newEngine(t, f)
	f.apps = seedApps(t, db, models.StoreAppStore, 4)

	f.perApp = func(ctx context.Context, rc *RunContext, app models.App) error {
		return errors.New("boom " + app.StoreAppID)
	}
	run, _ := e.Start(context.Background(), models.StoreAppStore, models.SyncDelta, models.TriggerManual, nil)
	e.Wait(models.StoreAppStore)

	got, _ := db.GetSyncRun(context.Background(), run.ID)
	if got.Status != models.SyncFailed {
		t.Fatalf("all apps failed → run failed, got %+v", got)
	}
	if got.Error == nil || *got.Error == "" {
		t.Fatal("no error recorded on the run")
	}
}
