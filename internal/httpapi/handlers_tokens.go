package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
)

func (a *API) mountTokens(r chi.Router) {
	r.Route("/me/tokens", func(r chi.Router) {
		r.Get("/", a.listTokens)
		r.Post("/", a.createToken)
		r.Delete("/{id}", a.revokeToken)
	})
}

func (a *API) listTokens(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	toks, err := a.DB.ListAPITokens(r.Context(), u.ID)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toks)
}

func (a *API) createToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		badRequest(w, err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeFieldErrs(w, map[string]string{"name": "give the token a name so you can recognise it later"})
		return
	}
	if len(body.Name) > 100 {
		writeFieldErrs(w, map[string]string{"name": "name is too long"})
		return
	}

	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		internalErr(w, err)
		return
	}
	u := auth.UserFromContext(r.Context())
	tok := &models.APIToken{
		UserID: u.ID,
		Name:   body.Name,
		Hash:   hash,
		Prefix: plaintext[:auth.PrefixLen],
	}
	if err := a.DB.CreateAPIToken(r.Context(), tok); err != nil {
		internalErr(w, err)
		return
	}
	// The only time the plaintext ever leaves the server.
	tok.Plaintext = plaintext
	writeJSON(w, http.StatusCreated, tok)
}

func (a *API) revokeToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		notFound(w)
		return
	}
	u := auth.UserFromContext(r.Context())
	if err := a.DB.RevokeAPIToken(r.Context(), id, u.ID); err != nil {
		if err == store.ErrNotFound {
			notFound(w)
			return
		}
		internalErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
