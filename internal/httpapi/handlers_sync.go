package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/permissions"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

// SyncDeps wires the engine + scheduler into the API.
type SyncDeps struct {
	Engine    *osync.Engine
	Scheduler *osync.Scheduler
}

func (s SyncDeps) MountRoutes(r chi.Router, a *API) {
	h := &syncHandlers{API: a, SyncDeps: s}
	r.Get("/sync/status", h.status)
	r.Get("/sync/runs", h.list)
	r.Get("/sync/runs/{id}", h.get)
	r.Get("/sync/runs/{id}/logs", h.logs)
	r.With(a.require(permissions.RunSync)).Post("/sync/runs", h.start)
	r.With(a.require(permissions.RunSync)).Post("/sync/runs/{id}/cancel", h.cancel)
	r.With(a.require(permissions.RunSync)).Post("/sync/reset", h.reset)
}

type syncHandlers struct {
	*API
	SyncDeps
}

type storeStatus struct {
	Store      models.Store    `json:"store"`
	Configured bool            `json:"configured"`
	Running    *models.SyncRun `json:"running"`
	Last       *models.SyncRun `json:"last"`
}

func (h *syncHandlers) status(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	stores := map[string]storeStatus{}
	for _, st := range []models.Store{models.StoreAppStore, models.StoreGooglePlay} {
		ss := storeStatus{Store: st, Configured: h.Engine.Configured(st)}
		if run := h.Engine.Running(st); run != nil {
			fresh, err := h.DB.GetSyncRun(r.Context(), run.ID)
			if err == nil {
				ss.Running = fresh
			}
		}
		if last, err := h.DB.LastSyncRun(r.Context(), st); err == nil {
			ss.Last = last
		}
		stores[string(st)] = ss
	}
	out["stores"] = stores
	if n := h.Scheduler.NextRun(); !n.IsZero() {
		out["next_scheduled"] = n.Format(time.RFC3339)
	} else {
		out["next_scheduled"] = nil
	}
	// unassigned apps hint
	if apps, err := h.DB.ListApps(r.Context(), storeAppFilterUnassigned()); err == nil {
		out["unassigned_apps"] = len(apps)
	}
	writeJSON(w, 200, out)
}

func (h *syncHandlers) list(w http.ResponseWriter, r *http.Request) {
	st := models.Store(r.URL.Query().Get("store"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	runs, err := h.DB.ListSyncRuns(r.Context(), st, 50, (page-1)*50)
	if err != nil {
		internalErr(w, err)
		return
	}
	if runs == nil {
		runs = []models.SyncRun{}
	}
	writeJSON(w, 200, runs)
}

func (h *syncHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	run, err := h.DB.GetSyncRun(r.Context(), id)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, run)
}

func (h *syncHandlers) logs(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	logs, err := h.DB.ListSyncLogs(r.Context(), id, after, 500)
	if err != nil {
		internalErr(w, err)
		return
	}
	if logs == nil {
		logs = []models.SyncRunLog{}
	}
	writeJSON(w, 200, logs)
}

func (h *syncHandlers) start(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Store models.Store    `json:"store"`
		Mode  models.SyncMode `json:"mode"`
	}
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	if !in.Store.Valid() || !in.Mode.Valid() {
		writeFieldErrs(w, map[string]string{"store": "appstore|googleplay", "mode": "full|delta"})
		return
	}
	u := auth.UserFromContext(r.Context())
	run, err := h.Engine.Start(r.Context(), in.Store, in.Mode, models.TriggerManual, &u.ID)
	switch {
	case errors.Is(err, osync.ErrAlreadyRunning):
		writeErr(w, 409, "sync_already_running", "a sync is already running for this store")
	case errors.Is(err, osync.ErrNotConfigured):
		writeErr(w, 409, "store_not_configured", "this store has no credentials configured")
	case err != nil:
		internalErr(w, err)
	default:
		writeJSON(w, 201, run)
	}
}

func (h *syncHandlers) cancel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	run, err := h.DB.GetSyncRun(r.Context(), id)
	if err != nil {
		internalErr(w, err)
		return
	}
	if err := h.Engine.Cancel(run.Store); errors.Is(err, osync.ErrNotRunning) {
		writeErr(w, 409, "not_running", "this run is not running")
		return
	}
	writeJSON(w, 202, map[string]any{"cancelling": true})
}

func (h *syncHandlers) reset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Store   models.Store `json:"store"`
		Confirm string       `json:"confirm"`
	}
	if err := decode(r, &in); err != nil || !in.Store.Valid() {
		badRequest(w, "store required")
		return
	}
	if in.Confirm != string(in.Store) {
		writeFieldErrs(w, map[string]string{"confirm": "type the store name to confirm"})
		return
	}
	if h.Engine.Running(in.Store) != nil {
		writeErr(w, 409, "sync_already_running", "cancel the running sync first")
		return
	}
	if err := h.DB.DeleteStoreData(r.Context(), in.Store); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}
