package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

type bootstrapAdminRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h Handler) BootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	expected := strings.TrimSpace(h.BootstrapAdminToken)
	provided := r.Header.Get("X-Bootstrap-Token")
	// S-NEW-1: constant-time comparison. Otherwise timings reveal the token byte by byte.
	if expected == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}
	var adminCount int64
	if err := h.DB.QueryRow(r.Context(), `select count(*) from users where role = 'admin' and deleted_at is null`).Scan(&adminCount); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if adminCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "admin already exists"})
		return
	}
	var req bootstrapAdminRequest
	if err := decodeJSONStrict(w, r, 32<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "email is required"})
		return
	}
	passHash, err := hashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var id int64
	err = h.DB.QueryRow(r.Context(), `
		insert into users (telegram_id, telegram_username, accepted_tos, email, password_hash, role, status)
		values (null, $1, true, $2, $3, 'admin', 'active')
		returning id`, "bootstrap_admin", email, passHash).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "userId": id})
}
