// Package settings gives typed, validated access to the settings table and
// notifies listeners (the scheduler) on change.
package settings

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
)

const (
	KeyScheduleEnabled = "sync.schedule.enabled"
	KeyScheduleTime    = "sync.schedule.time"
	KeyScheduleStores  = "sync.schedule.stores"
	KeyOverlapDays     = "sync.delta.overlap_days"
	KeyRetentionDays   = "metrics.retention_days"
	KeyDefaultRange    = "ui.default_range_days"
	KeyProductsSuggest = "products.suggest"
)

// ViewerKeys are readable by viewers; everything else is admin-only.
var ViewerKeys = map[string]bool{KeyDefaultRange: true}

var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

type Service struct {
	db        *store.DB
	mu        sync.RWMutex
	listeners []func(map[string]string)
}

func New(db *store.DB) *Service { return &Service{db: db} }

func (s *Service) OnChange(fn func(map[string]string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

func (s *Service) All(ctx context.Context) (map[string]string, error) { return s.db.GetSettings(ctx) }

// Validate checks a partial update. Returns field errors keyed by setting.
func Validate(kv map[string]string) map[string]string {
	errs := map[string]string{}
	for k, v := range kv {
		v = strings.TrimSpace(v)
		switch k {
		case KeyScheduleEnabled, KeyProductsSuggest:
			if v != "true" && v != "false" {
				errs[k] = "must be true or false"
			}
		case KeyScheduleTime:
			if !timeRe.MatchString(v) {
				errs[k] = "must be HH:MM"
			}
		case KeyScheduleStores:
			for _, st := range strings.Split(v, ",") {
				st = strings.TrimSpace(st)
				if st != "" && !models.Store(st).Valid() {
					errs[k] = fmt.Sprintf("unknown store %q", st)
				}
			}
		case KeyOverlapDays:
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 14 {
				errs[k] = "must be 1–14"
			}
		case KeyRetentionDays:
			if n, err := strconv.Atoi(v); err != nil || n < 0 {
				errs[k] = "must be 0 or a positive number of days"
			}
		case KeyDefaultRange:
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 3650 {
				errs[k] = "must be 1–3650"
			}
		default:
			errs[k] = "unknown setting"
		}
	}
	return errs
}

func (s *Service) Update(ctx context.Context, kv map[string]string) (map[string]string, error) {
	clean := make(map[string]string, len(kv))
	for k, v := range kv {
		clean[k] = strings.TrimSpace(v)
	}
	if errs := Validate(clean); len(errs) > 0 {
		return errs, nil
	}
	if err := s.db.SetSettings(ctx, clean); err != nil {
		return nil, err
	}
	all, err := s.db.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	ls := append([]func(map[string]string){}, s.listeners...)
	s.mu.RUnlock()
	for _, fn := range ls {
		fn(all)
	}
	return nil, nil
}

// typed getters --------------------------------------------------------------

func Bool(m map[string]string, key string) bool { return m[key] == "true" }

func Int(m map[string]string, key string, def int) int {
	n, err := strconv.Atoi(m[key])
	if err != nil {
		return def
	}
	return n
}

func Stores(m map[string]string) []models.Store {
	var out []models.Store
	for _, s := range strings.Split(m[KeyScheduleStores], ",") {
		s = strings.TrimSpace(s)
		if models.Store(s).Valid() {
			out = append(out, models.Store(s))
		}
	}
	return out
}

// ScheduleHM returns hour and minute of the daily schedule.
func ScheduleHM(m map[string]string) (int, int) {
	parts := strings.Split(m[KeyScheduleTime], ":")
	if len(parts) != 2 {
		return 6, 30
	}
	h, _ := strconv.Atoi(parts[0])
	mi, _ := strconv.Atoi(parts[1])
	return h, mi
}
