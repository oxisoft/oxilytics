package sync

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/store"
)

// Scheduler runs the daily delta sync and the nightly maintenance job.
type Scheduler struct {
	engine    *Engine
	db        *store.DB
	settings  *settings.Service
	tz        *time.Location
	backupDir string
	keep      int

	mu      sync.Mutex
	cron    *cron.Cron
	syncID  cron.EntryID
	nextRun time.Time
}

func NewScheduler(e *Engine, db *store.DB, st *settings.Service, tz *time.Location, backupDir string, keep int) *Scheduler {
	s := &Scheduler{engine: e, db: db, settings: st, tz: tz, backupDir: backupDir, keep: keep}
	s.cron = cron.New(cron.WithLocation(tz))
	return s
}

func (s *Scheduler) Start(ctx context.Context) error {
	all, err := s.settings.All(ctx)
	if err != nil {
		return err
	}
	s.apply(all)
	s.settings.OnChange(s.apply)
	if _, err := s.cron.AddFunc("15 3 * * *", s.maintenance); err != nil {
		return err
	}
	s.cron.Start()
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// apply (re)installs the daily sync entry from settings.
func (s *Scheduler) apply(all map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncID != 0 {
		s.cron.Remove(s.syncID)
		s.syncID = 0
		s.nextRun = time.Time{}
	}
	if !settings.Bool(all, settings.KeyScheduleEnabled) {
		slog.Info("scheduled sync disabled")
		return
	}
	h, m := settings.ScheduleHM(all)
	stores := settings.Stores(all)
	spec := fmt.Sprintf("%d %d * * *", m, h)
	id, err := s.cron.AddFunc(spec, func() { s.tick(stores) })
	if err != nil {
		slog.Error("schedule sync", "spec", spec, "err", err)
		return
	}
	s.syncID = id
	slog.Info("scheduled sync", "at", fmt.Sprintf("%02d:%02d", h, m), "tz", s.tz.String(), "stores", stores)
}

// NextRun returns the next scheduled sync time, or zero when disabled.
func (s *Scheduler) NextRun() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncID == 0 {
		return time.Time{}
	}
	return s.cron.Entry(s.syncID).Next
}

func (s *Scheduler) tick(stores []models.Store) {
	for _, st := range stores {
		if !s.engine.Configured(st) {
			continue
		}
		run, err := s.engine.Start(context.Background(), st, models.SyncDelta, models.TriggerSchedule, nil)
		if err != nil {
			slog.Warn("scheduled sync not started", "store", st, "err", err)
			continue
		}
		slog.Info("scheduled sync started", "store", st, "run", run.ID)
	}
}

// maintenance: backup, metric retention, sync-run pruning.
func (s *Scheduler) maintenance() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if s.backupDir != "" {
		if err := s.Backup(ctx); err != nil {
			slog.Error("backup", "err", err)
		}
	}
	all, err := s.settings.All(ctx)
	if err == nil {
		if days := settings.Int(all, settings.KeyRetentionDays, 0); days > 0 {
			cut := time.Now().In(s.tz).AddDate(0, 0, -days).Format("2006-01-02")
			if n, err := s.db.PruneMetrics(ctx, cut); err != nil {
				slog.Error("prune metrics", "err", err)
			} else if n > 0 {
				slog.Info("pruned metrics", "rows", n, "before", cut)
			}
		}
	}
	if err := s.db.PruneSyncRuns(ctx, time.Now().AddDate(0, 0, -90)); err != nil {
		slog.Error("prune sync runs", "err", err)
	}
}

// Backup writes a VACUUM INTO copy and rotates old ones.
func (s *Scheduler) Backup(ctx context.Context) error {
	if err := os.MkdirAll(s.backupDir, 0o750); err != nil {
		return err
	}
	name := filepath.Join(s.backupDir, "oxilytics-"+time.Now().In(s.tz).Format("20060102-150405")+".db")
	if err := s.db.BackupTo(ctx, name); err != nil {
		return err
	}
	slog.Info("backup written", "file", name)
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "oxilytics-") && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for len(files) > s.keep && s.keep > 0 {
		_ = os.Remove(filepath.Join(s.backupDir, files[0]))
		files = files[1:]
	}
	return nil
}
