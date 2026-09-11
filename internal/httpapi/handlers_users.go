package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/permissions"
)

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := a.DB.ListUsers(r.Context())
	if err != nil {
		internalErr(w, err)
		return
	}
	if us == nil {
		us = []models.User{}
	}
	writeJSON(w, 200, us)
}

type userIn struct {
	Email    string      `json:"email"`
	Name     string      `json:"name"`
	Role     models.Role `json:"role"`
	Password string      `json:"password"`
	Disabled *bool       `json:"disabled"`
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var in userIn
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	fields := map[string]string{}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if !strings.Contains(in.Email, "@") {
		fields["email"] = "valid e-mail required"
	}
	if in.Name == "" {
		fields["name"] = "required"
	}
	if !in.Role.Valid() {
		fields["role"] = "admin or viewer"
	}
	if msg := passwordPolicy(in.Password); msg != "" {
		fields["password"] = msg
	}
	if len(fields) > 0 {
		writeFieldErrs(w, fields)
		return
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		internalErr(w, err)
		return
	}
	u := &models.User{Email: in.Email, Name: in.Name, Role: in.Role, PasswordHash: h}
	if err := a.DB.CreateUser(r.Context(), u); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 201, u)
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	target, err := a.DB.GetUser(r.Context(), id)
	if err != nil {
		internalErr(w, err)
		return
	}
	var in userIn
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	name := target.Name
	if strings.TrimSpace(in.Name) != "" {
		name = strings.TrimSpace(in.Name)
	}
	role := target.Role
	if in.Role != "" {
		if !in.Role.Valid() {
			writeFieldErrs(w, map[string]string{"role": "admin or viewer"})
			return
		}
		role = in.Role
	}
	disabled := target.Disabled
	if in.Disabled != nil {
		disabled = *in.Disabled
	}
	admins, err := a.DB.CountAdmins(r.Context())
	if err != nil {
		internalErr(w, err)
		return
	}
	if !permissions.CanDemoteOrDisable(actor, target, role, disabled, admins) {
		writeErr(w, 409, "last_admin", "cannot remove the last active admin or demote yourself")
		return
	}
	if err := a.DB.UpdateUser(r.Context(), id, name, role, disabled); err != nil {
		internalErr(w, err)
		return
	}
	u, _ := a.DB.GetUser(r.Context(), id)
	writeJSON(w, 200, u)
}

func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		badRequest(w, "invalid json")
		return
	}
	if msg := passwordPolicy(in.Password); msg != "" {
		writeFieldErrs(w, map[string]string{"password": msg})
		return
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		internalErr(w, err)
		return
	}
	if err := a.DB.UpdateUserPassword(r.Context(), id, h); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	target, err := a.DB.GetUser(r.Context(), id)
	if err != nil {
		internalErr(w, err)
		return
	}
	admins, err := a.DB.CountAdmins(r.Context())
	if err != nil {
		internalErr(w, err)
		return
	}
	if !permissions.CanDeleteUser(actor, target, admins) {
		writeErr(w, 409, "last_admin", "cannot delete yourself or the last active admin")
		return
	}
	if err := a.DB.DeleteUser(r.Context(), id); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 204, nil)
}
