package httpapi

import "github.com/go-chi/chi/v5"

// Stubs replaced by later milestones.

func (a *API) mountProducts(r chi.Router) {
	r.HandleFunc("/products", notImplemented)
	r.HandleFunc("/products/*", notImplemented)
}

func (a *API) mountApps(r chi.Router) {
	r.HandleFunc("/apps", notImplemented)
	r.HandleFunc("/apps/*", notImplemented)
}

func (a *API) mountMetrics(r chi.Router) {
	r.HandleFunc("/metrics/*", notImplemented)
}

func (a *API) mountReviews(r chi.Router) {
	r.HandleFunc("/reviews", notImplemented)
	r.HandleFunc("/reviews/*", notImplemented)
}
