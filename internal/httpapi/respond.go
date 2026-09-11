// Package httpapi: chi routes + handlers. Validate, authorise, call a service,
// respond JSON. No SQL here.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/oxisoft/oxilytics/internal/store"
)

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type errEnvelope struct {
	Error apiError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errEnvelope{apiError{Code: code, Message: msg}})
}

func writeFieldErrs(w http.ResponseWriter, fields map[string]string) {
	writeJSON(w, http.StatusBadRequest, errEnvelope{apiError{Code: "validation", Message: "invalid input", Fields: fields}})
}

func badRequest(w http.ResponseWriter, msg string) { writeErr(w, 400, "bad_request", msg) }
func unauthorized(w http.ResponseWriter)          { writeErr(w, 401, "unauthenticated", "sign in required") }
func forbidden(w http.ResponseWriter)             { writeErr(w, 403, "forbidden", "not allowed") }
func notFound(w http.ResponseWriter)              { writeErr(w, 404, "not_found", "not found") }

// internalErr maps store errors to HTTP and logs everything else.
func internalErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		notFound(w)
	case errors.Is(err, store.ErrConflict):
		writeErr(w, 409, "conflict", "already exists")
	default:
		slog.Error("internal error", "err", err)
		writeErr(w, 500, "internal", "internal error")
	}
}

// decode reads a JSON body of at most 1 MiB.
func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
