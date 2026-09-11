package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/permissions"
	"github.com/oxisoft/oxilytics/internal/store"
)

func storeAppFilterUnassigned() store.AppFilter { return store.AppFilter{Unassigned: true} }

// products -------------------------------------------------------------------

func (a *API) mountProducts(r chi.Router) {
	r.Get("/products", a.listProducts)
	r.Get("/products/{id}", a.getProduct)
	r.Group(func(r chi.Router) {
		r.Use(a.require(permissions.ManageProducts))
		r.Post("/products", a.createProduct)
		r.Put("/products/{id}", a.updateProduct)
		r.Delete("/products/{id}", a.deleteProduct)
		r.Post("/products/{id}/apps", a.linkApp)
		r.Delete("/products/{id}/apps/{appId}", a.unlinkApp)
		r.Get("/products-suggestions", a.suggestions)
		r.Post("/products-suggestions/accept", a.acceptSuggestion)
	})
}

func (a *API) listProducts(w http.ResponseWriter, r *http.Request) {
	from, to, prevFrom, prevTo := rangeParams(r)
	rows, err := a.DB.ProductRows(r.Context(), from, to, prevFrom, prevTo, r.URL.Query().Get("archived") == "1")
	if err != nil {
		internalErr(w, err)
		return
	}
	if rows == nil {
		rows = []store.ProductRow{}
	}
	writeJSON(w, 200, rows)
}

func (a *API) getProduct(w http.ResponseWriter, r *http.Request) {
	p, err := a.productParam(r)
	if err != nil {
		internalErr(w, err)
		return
	}
	apps, err := a.DB.ListApps(r.Context(), store.AppFilter{ProductID: &p.ID})
	if err != nil {
		internalErr(w, err)
		return
	}
	if apps == nil {
		apps = []models.App{}
	}
	p.Apps = apps
	writeJSON(w, 200, p)
}

// productParam resolves {id} as numeric id or slug.
func (a *API) productParam(r *http.Request) (*models.Product, error) {
	raw := chi.URLParam(r, "id")
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return a.DB.GetProduct(r.Context(), id)
	}
	return a.DB.GetProductBySlug(r.Context(), raw)
}

type productIn struct {
	Name        string  `json:"name"`
	IconURL     *string `json:"icon_url"`
	Description *string `json:"description"`
	Archived    *bool   `json:"archived"`
}

func (a *API) createProduct(w http.ResponseWriter, r *http.Request) {
	var in productIn
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 {
		writeFieldErrs(w, map[string]string{"name": "1–120 characters"})
		return
	}
	p := &models.Product{Name: in.Name, IconURL: in.IconURL, Description: in.Description}
	if err := a.DB.CreateProduct(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeFieldErrs(w, map[string]string{"name": "a product with this name exists"})
			return
		}
		internalErr(w, err)
		return
	}
	p.Apps = []models.App{}
	writeJSON(w, 201, p)
}

func (a *API) updateProduct(w http.ResponseWriter, r *http.Request) {
	p, err := a.productParam(r)
	if err != nil {
		internalErr(w, err)
		return
	}
	var in productIn
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	name := p.Name
	if strings.TrimSpace(in.Name) != "" {
		name = strings.TrimSpace(in.Name)
	}
	icon, desc, archived := p.IconURL, p.Description, p.Archived
	if in.IconURL != nil {
		icon = in.IconURL
	}
	if in.Description != nil {
		desc = in.Description
	}
	if in.Archived != nil {
		archived = *in.Archived
	}
	if err := a.DB.UpdateProduct(r.Context(), p.ID, name, icon, desc, archived); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeFieldErrs(w, map[string]string{"name": "a product with this name exists"})
			return
		}
		internalErr(w, err)
		return
	}
	fresh, _ := a.DB.GetProduct(r.Context(), p.ID)
	writeJSON(w, 200, fresh)
}

func (a *API) deleteProduct(w http.ResponseWriter, r *http.Request) {
	p, err := a.productParam(r)
	if err != nil {
		internalErr(w, err)
		return
	}
	if err := a.DB.DeleteProduct(r.Context(), p.ID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, 409, "has_apps", "unlink the store apps first")
			return
		}
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func (a *API) linkApp(w http.ResponseWriter, r *http.Request) {
	p, err := a.productParam(r)
	if err != nil {
		internalErr(w, err)
		return
	}
	var in struct {
		AppID int64 `json:"app_id"`
	}
	if err := decode(r, &in); err != nil || in.AppID == 0 {
		badRequest(w, "app_id required")
		return
	}
	app, err := a.DB.GetApp(r.Context(), in.AppID)
	if err != nil {
		internalErr(w, err)
		return
	}
	if app.IgnoredAt != nil {
		writeErr(w, 409, "app_ignored", "restore the app before linking it")
		return
	}
	if err := a.DB.LinkApp(r.Context(), in.AppID, p.ID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, 409, "platform_taken", "this product already has a "+string(app.Platform)+" listing")
			return
		}
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func (a *API) unlinkApp(w http.ResponseWriter, r *http.Request) {
	appID, _ := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err := a.DB.UnlinkApp(r.Context(), appID); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

type suggestion struct {
	App              models.App      `json:"app"`
	SuggestedProduct *models.Product `json:"suggested_product"`
	ProposedName     string          `json:"proposed_name"`
}

func (a *API) suggestions(w http.ResponseWriter, r *http.Request) {
	apps, err := a.DB.ListApps(r.Context(), store.AppFilter{Unassigned: true})
	if err != nil {
		internalErr(w, err)
		return
	}
	out := []suggestion{}
	for _, app := range apps {
		s := suggestion{App: app, ProposedName: app.Name}
		if app.SuggestedProductID != nil {
			if p, err := a.DB.GetProduct(r.Context(), *app.SuggestedProductID); err == nil {
				s.SuggestedProduct = p
			}
		}
		out = append(out, s)
	}
	writeJSON(w, 200, out)
}

func (a *API) acceptSuggestion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AppID     int64  `json:"app_id"`
		ProductID *int64 `json:"product_id"`
		Name      string `json:"name"`
	}
	if err := decode(r, &in); err != nil || in.AppID == 0 {
		badRequest(w, "app_id required")
		return
	}
	app, err := a.DB.GetApp(r.Context(), in.AppID)
	if err != nil {
		internalErr(w, err)
		return
	}
	pid := in.ProductID
	if pid == nil {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = app.Name
		}
		p := &models.Product{Name: name, IconURL: app.IconURL}
		if err := a.DB.CreateProduct(r.Context(), p); err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeFieldErrs(w, map[string]string{"name": "a product with this name exists — pick it instead"})
				return
			}
			internalErr(w, err)
			return
		}
		pid = &p.ID
	}
	if err := a.DB.LinkApp(r.Context(), app.ID, *pid); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, 409, "platform_taken", "that product already has a "+string(app.Platform)+" listing")
			return
		}
		internalErr(w, err)
		return
	}
	p, _ := a.DB.GetProduct(r.Context(), *pid)
	writeJSON(w, 200, p)
}

// store apps -----------------------------------------------------------------

func (a *API) mountApps(r chi.Router) {
	r.Get("/apps", a.listApps)
	r.With(a.require(permissions.ViewIgnoredApps)).Get("/apps-ignored", a.listIgnored)
	r.Get("/apps/{id}", a.getApp)
	r.Group(func(r chi.Router) {
		r.Use(a.require(permissions.IgnoreApps))
		r.Put("/apps/{id}", a.updateApp)
		r.Post("/apps/{id}/ignore", a.ignoreApp)
		r.Post("/apps/{id}/restore", a.restoreApp)
		r.Put("/apps/{id}/ignore-reason", a.ignoreReason)
	})
}

func (a *API) listApps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AppFilter{Store: models.Store(q.Get("store")), Platform: models.Platform(q.Get("platform")), Unassigned: q.Get("unassigned") == "1"}
	if v := q.Get("product_id"); v != "" {
		id, _ := strconv.ParseInt(v, 10, 64)
		f.ProductID = &id
	}
	apps, err := a.DB.ListApps(r.Context(), f)
	if err != nil {
		internalErr(w, err)
		return
	}
	if apps == nil {
		apps = []models.App{}
	}
	// 30-day downloads per app
	from, to, _, _ := rangeParams(r)
	type row struct {
		models.App
		Downloads int64 `json:"downloads"`
	}
	out := make([]row, 0, len(apps))
	for _, app := range apps {
		id := app.ID
		t, _ := a.DB.SumTotals(r.Context(), store.Scope{AppID: &id}, from, to)
		out = append(out, row{App: app, Downloads: t.Downloads})
	}
	writeJSON(w, 200, out)
}

func (a *API) getApp(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	app, err := a.DB.GetApp(r.Context(), id)
	if err != nil {
		internalErr(w, err)
		return
	}
	if app.IgnoredAt != nil && !permissions.Can(auth.UserFromContext(r.Context()), permissions.ViewIgnoredApps) {
		notFound(w)
		return
	}
	writeJSON(w, 200, app)
}

func (a *API) updateApp(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		Name    string  `json:"name"`
		IconURL *string `json:"icon_url"`
	}
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeFieldErrs(w, map[string]string{"name": "required"})
		return
	}
	if err := a.DB.UpdateAppMeta(r.Context(), id, in.Name, in.IconURL); err != nil {
		internalErr(w, err)
		return
	}
	app, _ := a.DB.GetApp(r.Context(), id)
	writeJSON(w, 200, app)
}

func (a *API) ignoreApp(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		Reason *string `json:"reason"`
	}
	_ = decode(r, &in)
	u := auth.UserFromContext(r.Context())
	if err := a.DB.IgnoreApp(r.Context(), id, u.ID, in.Reason); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func (a *API) restoreApp(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.DB.RestoreApp(r.Context(), id); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func (a *API) ignoreReason(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		Reason *string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	if err := a.DB.UpdateAppIgnoreReason(r.Context(), id, in.Reason); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func (a *API) listIgnored(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.ListIgnoredApps(r.Context())
	if err != nil {
		internalErr(w, err)
		return
	}
	if rows == nil {
		rows = []store.IgnoredAppRow{}
	}
	writeJSON(w, 200, rows)
}
