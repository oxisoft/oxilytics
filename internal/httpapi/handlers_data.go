package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// SyncAPI is implemented by the sync engine (M3). nil → routes answer 501.
type SyncAPI interface {
	MountRoutes(r chi.Router, a *API)
}

// mountData registers the data routes (products, apps, metrics, reviews, sync)
// under the setup gate. Milestones add their handlers here.
func (a *API) mountData(r chi.Router) {
	if a.Sync != nil {
		a.Sync.MountRoutes(r, a)
	} else {
		r.HandleFunc("/sync/*", notImplemented)
		r.HandleFunc("/sync", notImplemented)
	}
	a.mountProducts(r)
	a.mountApps(r)
	a.mountMetrics(r)
	a.mountReviews(r)
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	writeErr(w, http.StatusNotImplemented, "not_implemented", "not yet available")
}
