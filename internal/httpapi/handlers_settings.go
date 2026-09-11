package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/permissions"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/setup"
)

// StoreTester performs a live connection test against a store.
type StoreTester interface {
	Test(ctx context.Context, store models.Store) setup.TestResult
}

// settings -------------------------------------------------------------------

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.All(r.Context())
	if err != nil {
		internalErr(w, err)
		return
	}
	u := auth.UserFromContext(r.Context())
	if !permissions.Can(u, permissions.EditSettings) {
		filtered := map[string]string{}
		for k := range settings.ViewerKeys {
			filtered[k] = all[k]
		}
		all = filtered
	}
	writeJSON(w, 200, all)
}

func (a *API) putSettings(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if err := decode(r, &in); err != nil {
		badRequest(w, "expected an object of string values")
		return
	}
	if len(in) == 0 {
		badRequest(w, "nothing to update")
		return
	}
	fields, err := a.Settings.Update(r.Context(), in)
	if err != nil {
		internalErr(w, err)
		return
	}
	if len(fields) > 0 {
		writeFieldErrs(w, fields)
		return
	}
	all, _ := a.Settings.All(r.Context())
	writeJSON(w, 200, all)
}

// setup ----------------------------------------------------------------------

func (a *API) setupStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, a.Setup)
}

func (a *API) setupGuide(w http.ResponseWriter, r *http.Request) {
	st := models.Store(chi.URLParam(r, "store"))
	if !st.Valid() {
		notFound(w)
		return
	}
	ss := a.Setup.Stores[st]
	writeJSON(w, 200, map[string]any{
		"store":      st,
		"configured": ss.Configured,
		"info":       ss.Info,
		"env":        setup.EnvVars(st),
	})
}

func (a *API) setupTest(w http.ResponseWriter, r *http.Request) {
	st := models.Store(chi.URLParam(r, "store"))
	if !st.Valid() {
		notFound(w)
		return
	}
	if !a.Setup.Configured(st) {
		writeErr(w, 409, "store_not_configured", "provide credentials and restart first")
		return
	}
	if a.Tester == nil {
		writeErr(w, 501, "not_implemented", "connection test not available")
		return
	}
	writeJSON(w, 200, a.Tester.Test(r.Context(), st))
}
