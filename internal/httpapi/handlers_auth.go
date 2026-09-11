package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/version"
)

// system ---------------------------------------------------------------------

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	dbState := "ok"
	status := 200
	if err := a.DB.Ping(r.Context()); err != nil {
		dbState = "error"
		status = 503
	}
	writeJSON(w, status, map[string]any{"ok": status == 200, "db": dbState, "setup_required": a.Setup.SetupRequired})
}

func (a *API) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"version": version.Version, "commit": version.Commit, "short_commit": version.ShortCommit(), "build_time": version.BuildTime, "go": version.GoVersion, "display": version.String()})
}

// auth -----------------------------------------------------------------------

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in loginReq
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" || in.Password == "" {
		writeFieldErrs(w, map[string]string{"email": "required", "password": "required"})
		return
	}
	u, err := a.Auth.Login(w, r, in.Email, in.Password)
	switch {
	case errors.Is(err, auth.ErrTOTPRequired):
		writeJSON(w, http.StatusAccepted, map[string]any{"totp_required": true})
	case errors.Is(err, auth.ErrRateLimited):
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in a minute")
	case errors.Is(err, auth.ErrDisabled):
		writeErr(w, http.StatusForbidden, "disabled", "account disabled")
	case err != nil:
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	default:
		writeJSON(w, 200, map[string]any{"user": u})
	}
}

func (a *API) loginTOTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Code) == "" {
		badRequest(w, "code required")
		return
	}
	u, err := a.Auth.LoginTOTP(w, r, in.Code)
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many attempts")
	case err != nil:
		writeErr(w, http.StatusUnauthorized, "invalid_code", "invalid code")
	default:
		writeJSON(w, 200, map[string]any{"user": u})
	}
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	a.Auth.Logout(w, r)
	writeJSON(w, 204, nil)
}

// me -------------------------------------------------------------------------

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, auth.UserFromContext(r.Context()))
}

func (a *API) updateMe(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeFieldErrs(w, map[string]string{"name": "1–100 characters"})
		return
	}
	if err := a.DB.UpdateUserName(r.Context(), u.ID, in.Name); err != nil {
		internalErr(w, err)
		return
	}
	u.Name = in.Name
	writeJSON(w, 200, u)
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, in.Current) {
		writeFieldErrs(w, map[string]string{"current": "wrong password"})
		return
	}
	if msg := passwordPolicy(in.New); msg != "" {
		writeFieldErrs(w, map[string]string{"new": msg})
		return
	}
	h, err := auth.HashPassword(in.New)
	if err != nil {
		internalErr(w, err)
		return
	}
	if err := a.DB.UpdateUserPassword(r.Context(), u.ID, h); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func (a *API) totpSetup(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if u.TOTPEnabled {
		writeErr(w, 409, "totp_enabled", "two-factor is already enabled")
		return
	}
	s, err := a.Auth.GenerateTOTP(u.Email)
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, s)
}

func (a *API) totpEnable(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var in struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if err := decode(r, &in); err != nil || in.Secret == "" || in.Code == "" {
		badRequest(w, "secret and code required")
		return
	}
	if u.TOTPEnabled {
		writeErr(w, 409, "totp_enabled", "two-factor is already enabled")
		return
	}
	codes, err := a.Auth.EnableTOTP(r.Context(), u, in.Secret, in.Code)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeFieldErrs(w, map[string]string{"code": "wrong code"})
		return
	}
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"recovery_codes": codes})
}

func (a *API) totpDisable(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var in struct {
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	if err := a.Auth.DisableTOTP(r.Context(), u, in.Password); errors.Is(err, auth.ErrInvalidCredentials) {
		writeFieldErrs(w, map[string]string{"password": "wrong password"})
		return
	} else if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func passwordPolicy(pw string) string {
	if len(pw) < 10 {
		return "at least 10 characters"
	}
	if len(pw) > 200 {
		return "too long"
	}
	return ""
}
