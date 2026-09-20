// Package sync runs store synchronisations: full / delta, manual / scheduled,
// one run per store at a time, cancellable, with progress and logs in the DB.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/store"
)

var (
	ErrAlreadyRunning = errors.New("sync already running for this store")
	ErrNotConfigured  = errors.New("store not configured")
	ErrNotRunning     = errors.New("no running sync")
)

// Ingester is implemented per store (appstore, googleplay).
type Ingester interface {
	Store() models.Store
	// DiscoverApps upserts the catalogue and returns active apps.
	DiscoverApps(ctx context.Context, rc *RunContext) ([]models.App, error)
	// SyncApp ingests everything for one app in the run's range.
	SyncApp(ctx context.Context, rc *RunContext, app models.App) error
}

// Engine owns running syncs.
type Engine struct {
	db        *store.DB
	settings  *settings.Service
	ingesters map[models.Store]Ingester
	tz        *time.Location

	mu      sync.Mutex
	running map[models.Store]*runHandle
}

type runHandle struct {
	run    *models.SyncRun
	cancel context.CancelFunc
	done   chan struct{}
}

func NewEngine(db *store.DB, st *settings.Service, tz *time.Location, ingesters ...Ingester) *Engine {
	e := &Engine{db: db, settings: st, tz: tz, ingesters: map[models.Store]Ingester{}, running: map[models.Store]*runHandle{}}
	for _, in := range ingesters {
		e.ingesters[in.Store()] = in
	}
	return e
}

func (e *Engine) Configured(st models.Store) bool { _, ok := e.ingesters[st]; return ok }

// Recover marks runs left running by a previous process as interrupted.
func (e *Engine) Recover(ctx context.Context) error {
	n, err := e.db.MarkInterruptedRuns(ctx)
	if n > 0 {
		slog.Warn("marked interrupted sync runs", "count", n)
	}
	return err
}

// Start enqueues and immediately runs a sync in a goroutine.
func (e *Engine) Start(ctx context.Context, st models.Store, mode models.SyncMode, trigger models.SyncTrigger, requestedBy *int64) (*models.SyncRun, error) {
	in, ok := e.ingesters[st]
	if !ok {
		return nil, ErrNotConfigured
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, busy := e.running[st]; busy {
		return nil, ErrAlreadyRunning
	}
	run := &models.SyncRun{Store: st, Mode: mode, Trigger: trigger, RequestedBy: requestedBy}
	if err := e.db.CreateSyncRun(ctx, run); err != nil {
		return nil, err
	}
	rctx, cancel := context.WithCancel(context.Background())
	h := &runHandle{run: run, cancel: cancel, done: make(chan struct{})}
	e.running[st] = h
	go e.execute(rctx, in, h)
	return run, nil
}

// Cancel stops the running sync for a store.
func (e *Engine) Cancel(st models.Store) error {
	e.mu.Lock()
	h, ok := e.running[st]
	e.mu.Unlock()
	if !ok {
		return ErrNotRunning
	}
	h.cancel()
	return nil
}

// Running returns the in-flight run for a store, if any.
func (e *Engine) Running(st models.Store) *models.SyncRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	if h, ok := e.running[st]; ok {
		return h.run
	}
	return nil
}

// Wait blocks until the store's run finishes (tests).
func (e *Engine) Wait(st models.Store) {
	e.mu.Lock()
	h, ok := e.running[st]
	e.mu.Unlock()
	if ok {
		<-h.done
	}
}

// Stop cancels all runs and waits for them (shutdown).
func (e *Engine) Stop() {
	e.mu.Lock()
	hs := make([]*runHandle, 0, len(e.running))
	for _, h := range e.running {
		h.cancel()
		hs = append(hs, h)
	}
	e.mu.Unlock()
	for _, h := range hs {
		<-h.done
	}
}

// RunContext is passed to ingesters: range, checkpoints, logging, counters.
type RunContext struct {
	Run         *models.SyncRun
	DB          *store.DB
	Mode        models.SyncMode
	From, To    string // YYYY-MM-DD inclusive
	OverlapDays int
	Suggest     bool
	Log         func(level string, appID *int64, format string, args ...any)
	AddRows     func(metrics, reviews int64)
	Stats       *Stats
}

type Stats struct {
	mu        sync.Mutex
	APICalls  int64                 `json:"api_calls"`
	Bytes     int64                 `json:"bytes"`
	Apps      map[string]*AppStat   `json:"apps"`
	Steps     map[string]int64      `json:"step_ms"`
	Errors    int                   `json:"errors"`
}

type AppStat struct {
	Name    string `json:"name"`
	Metrics int64  `json:"metrics"`
	Reviews int64  `json:"reviews"`
	Errors  int    `json:"errors"`
	MS      int64  `json:"ms"`
}

func (s *Stats) app(id int64, name string) *AppStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := fmt.Sprint(id)
	if s.Apps[k] == nil {
		s.Apps[k] = &AppStat{Name: name}
	}
	return s.Apps[k]
}

func (s *Stats) AddBytes(n int64) { s.mu.Lock(); s.Bytes += n; s.APICalls++; s.mu.Unlock() }

func (e *Engine) execute(ctx context.Context, in Ingester, h *runHandle) {
	run := h.run
	st := in.Store()
	defer func() {
		e.mu.Lock()
		delete(e.running, st)
		e.mu.Unlock()
		close(h.done)
	}()
	bg := context.Background()
	log := func(level string, appID *int64, format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		slog.Log(bg, slogLevel(level), msg, "run", run.ID, "store", st)
		_ = e.db.AddSyncLog(bg, run.ID, level, appID, msg)
	}

	all, err := e.settings.All(bg)
	if err != nil {
		e.finish(run, models.SyncFailed, err, nil)
		return
	}
	overlap := settings.Int(all, settings.KeyOverlapDays, 3)
	rc := &RunContext{Run: run, DB: e.db, Mode: run.Mode, OverlapDays: overlap, Suggest: settings.Bool(all, settings.KeyProductsSuggest), Log: log,
		Stats: &Stats{Apps: map[string]*AppStat{}, Steps: map[string]int64{}}}

	today := time.Now().In(e.tz)
	rc.To = today.AddDate(0, 0, -1).Format("2006-01-02")
	if run.Mode == models.SyncFull {
		rc.From = "2008-07-10" // App Store launch; ingesters clamp to what the store has
	} else {
		rc.From = today.AddDate(0, 0, -overlap-1).Format("2006-01-02") // ingesters extend per-app from checkpoints
	}

	var rowsMetrics, rowsReviews int64
	var pmu sync.Mutex
	rc.AddRows = func(m, r int64) {
		pmu.Lock()
		rowsMetrics += m
		rowsReviews += r
		pmu.Unlock()
	}

	log("info", nil, "run %d started: %s %s, range %s..%s", run.ID, st, run.Mode, rc.From, rc.To)
	apps, err := in.DiscoverApps(ctx, rc)
	if err != nil {
		log("error", nil, "discover apps: %v", err)
		e.finish(run, statusFor(ctx, err), err, rc.Stats)
		return
	}
	if err := e.db.StartSyncRun(bg, run.ID, rc.From, rc.To, len(apps)); err != nil {
		e.finish(run, models.SyncFailed, err, rc.Stats)
		return
	}
	log("info", nil, "%d active apps", len(apps))

	failed := 0
	var appErrs []error
	var fatal error
	stopTicker := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				pmu.Lock()
				m, r := rowsMetrics, rowsReviews
				pmu.Unlock()
				_ = e.db.UpdateSyncProgress(bg, run.ID, run.AppsDone, m, r)
			case <-stopTicker:
				return
			}
		}
	}()

	for i, app := range apps {
		if ctx.Err() != nil {
			break
		}
		as := rc.Stats.app(app.ID, app.Name)
		t0 := time.Now()
		appID := app.ID
		log("info", &appID, "syncing %s (%s)", app.Name, app.StoreAppID)
		err := in.SyncApp(ctx, rc, app)
		as.MS = time.Since(t0).Milliseconds()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			as.Errors++
			rc.Stats.Errors++
			failed++
			appErrs = append(appErrs, err)
			log("error", &appID, "%s: %v", app.Name, err)
			if isFatal(err) {
				fatal = err
				break
			}
		} else {
			_ = e.db.TouchAppSynced(bg, app.ID)
		}
		run.AppsDone = i + 1
	}
	close(stopTicker)
	pmu.Lock()
	_ = e.db.UpdateSyncProgress(bg, run.ID, run.AppsDone, rowsMetrics, rowsReviews)
	pmu.Unlock()

	switch {
	case ctx.Err() != nil:
		log("warn", nil, "run cancelled after %d/%d apps", run.AppsDone, len(apps))
		e.finish(run, models.SyncCancelled, nil, rc.Stats)
	case fatal != nil:
		e.finish(run, models.SyncFailed, fatal, rc.Stats)
	case len(apps) > 0 && failed == len(apps):
		e.finish(run, models.SyncFailed, fmt.Errorf("all %d apps failed: %s", failed, summarizeErrors(appErrs)), rc.Stats)
	default:
		if failed > 0 {
			log("warn", nil, "finished with %d app errors", failed)
		}
		log("info", nil, "run finished: %d metric rows, %d reviews", rowsMetrics, rowsReviews)
		e.finish(run, models.SyncSucceeded, nil, rc.Stats)
	}
}

func (e *Engine) finish(run *models.SyncRun, status models.SyncStatus, err error, stats *Stats) {
	var errMsg, statsJSON *string
	if err != nil {
		s := err.Error()
		errMsg = &s
	}
	if stats != nil {
		stats.mu.Lock()
		b, _ := json.Marshal(stats)
		stats.mu.Unlock()
		s := string(b)
		statsJSON = &s
	}
	if ferr := e.db.FinishSyncRun(context.Background(), run.ID, status, errMsg, statsJSON); ferr != nil {
		slog.Error("finish sync run", "run", run.ID, "err", ferr)
	}
	run.Status = status
}

func statusFor(ctx context.Context, err error) models.SyncStatus {
	if ctx.Err() != nil {
		return models.SyncCancelled
	}
	return models.SyncFailed
}

// FatalError wraps errors that should abort the whole run (auth/config).
type FatalError struct{ Err error }

func (f *FatalError) Error() string { return f.Err.Error() }
func (f *FatalError) Unwrap() error { return f.Err }

func isFatal(err error) bool {
	var f *FatalError
	return errors.As(err, &f)
}

// summarizeErrors turns a pile of per-app failures into one line worth showing
// on the runs list. When every app failed for the same reason — the usual case
// for a schema or credential problem — that reason IS the headline, so report
// it verbatim instead of the useless "all N apps failed".
func summarizeErrors(errs []error) string {
	if len(errs) == 0 {
		return "no error recorded"
	}
	counts := map[string]int{}
	order := []string{}
	for _, err := range errs {
		msg := firstLine(err.Error())
		if _, seen := counts[msg]; !seen {
			order = append(order, msg)
		}
		counts[msg]++
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	if len(order) == 1 {
		return order[0]
	}
	parts := make([]string, 0, 3)
	for _, msg := range order[:min(len(order), 2)] {
		parts = append(parts, fmt.Sprintf("%s (%d apps)", msg, counts[msg]))
	}
	if len(order) > 2 {
		parts = append(parts, fmt.Sprintf("and %d more distinct errors", len(order)-2))
	}
	return strings.Join(parts, "; ")
}

// firstLine keeps an error summary to its headline: per-app errors join several
// failures with newlines, which would otherwise wreck a single-line table cell.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func slogLevel(l string) slog.Level {
	switch l {
	case "error":
		return slog.LevelError
	case "warn":
		return slog.LevelWarn
	case "debug":
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// helpers shared by ingesters -----------------------------------------------

// MaxDay returns the later of two YYYY-MM-DD strings ("" counts as earliest).
func MaxDay(a, b string) string {
	if a > b {
		return a
	}
	return b
}

// MinDay returns the earlier of two YYYY-MM-DD strings, ignoring "".
func MinDay(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// DeltaFrom computes the per-app start day: checkpoint − overlap, but never
// before rc.From's floor for delta runs when no checkpoint exists.
func DeltaFrom(rc *RunContext, checkpoint string) string {
	if rc.Mode == models.SyncFull || checkpoint == "" {
		if rc.Mode == models.SyncFull {
			return rc.From
		}
		// no checkpoint on a delta run: behave like a full run for this app
		return "2008-07-10"
	}
	t, err := time.Parse("2006-01-02", checkpoint)
	if err != nil {
		return rc.From
	}
	return t.AddDate(0, 0, -rc.OverlapDays).Format("2006-01-02")
}

// SuggestProduct attaches a suggestion for an unassigned app by exact
// normalised-name match. Never links.
func SuggestProduct(ctx context.Context, rc *RunContext, app *models.App) {
	if !rc.Suggest || app.ProductID != nil || app.SuggestedProductID != nil || app.IgnoredAt != nil {
		return
	}
	p, err := rc.DB.FindProductByNormalizedName(ctx, app.Name)
	if err != nil {
		return
	}
	_ = rc.DB.SetAppSuggestion(ctx, app.ID, &p.ID)
	rc.Log("info", &app.ID, "%s looks like product %q — confirm on the Products screen", app.Name, p.Name)
}
