package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	adminusers "mindstrata-stage1/api/internal/httpapi/admin/users"
)

const adminUsersListCacheTTL = 15 * time.Second

func (h Handler) AdminUsers(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminSection(w, r, "users", r.Method != http.MethodGet)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.adminListUsers(w, r)
	case http.MethodPost:
		h.adminCreateUser(w, r, user)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) adminListUsers(w http.ResponseWriter, r *http.Request) {
	limit := int64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && (parsed == 10 || parsed == 50 || parsed == 100) {
			limit = parsed
		}
	}
	offset := int64(0)
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	cacheKey := adminusers.ListCacheKey(q, limit, offset)
	if body, ok := h.c.adminUsersList.get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	where := "where deleted_at is null"
	filterArgs := []any{}
	if q != "" {
		where += " and (lower(coalesce(email, '')) like lower($1) or lower(coalesce(telegram_username, '')) like lower($1) or id::text = $2)"
		filterArgs = append(filterArgs, "%"+q+"%", q)
	}
	var total int64
	if err := h.DB.QueryRow(r.Context(), fmt.Sprintf(`select count(*) from users %s`, where), filterArgs...).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	args := append([]any{}, filterArgs...)
	limitPlaceholder := len(args) + 1
	offsetPlaceholder := len(args) + 2
	args = append(args, limit, offset)
	rows, err := h.DB.Query(r.Context(), fmt.Sprintf(`
		select u.id, coalesce(u.email, ''), u.role, u.status, coalesce(u.telegram_username, ''), u.created_at, u.last_login_at,
		       coalesce((select count(*) from users_dialogs d join dialogs_messages dm on dm.dialog_id = d.id where d.user_id = u.id), 0)
		from users u
		%s
		order by u.id desc
		limit $%d offset $%d`, strings.Replace(where, "deleted_at", "u.deleted_at", 1), limitPlaceholder, offsetPlaceholder), args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var email, role, status, username string
		var createdAt time.Time
		var lastLoginAt *time.Time
		var messageCount int64
		if err := rows.Scan(&id, &email, &role, &status, &username, &createdAt, &lastLoginAt, &messageCount); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, map[string]any{"id": id, "email": email, "role": role, "status": status, "telegramUsername": username, "createdAt": createdAt, "lastLoginAt": lastLoginAt, "messageCount": messageCount})
	}
	body, _ := json.Marshal(map[string]any{"ok": true, "users": out, "total": total, "limit": limit, "offset": offset})
	h.c.adminUsersList.set(cacheKey, body, adminUsersListCacheTTL)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (h Handler) adminCreateUser(w http.ResponseWriter, r *http.Request, actor AuthenticatedUser) {
	var req adminCreateUserRequest
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := decodeJSONStrictBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	email := normalizeEmail(req.Email)
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "user"
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}
	if email == "" || !validRole(role) || !validStatus(status) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid email, role or status"})
		return
	}
	// K-NEW5-1: privilege escalation: an admin cannot create a user
	// with the owner role. Symmetric to adminPatchUser (K-NEW-4).
	if role == "owner" && actor.Role != "owner" {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "only owner can create owner accounts"})
		return
	}
	password := strings.TrimSpace(req.Password)
	var passHash *string
	if password != "" {
		hashed, err := hashPassword(password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		passHash = &hashed
	}
	var id int64
	err := h.DB.QueryRow(r.Context(), `
		insert into users (telegram_id, telegram_username, accepted_tos, email, password_hash, role, status)
		values (null, $1, true, $2, $3, $4, $5)
		returning id`, "manual_"+strings.ReplaceAll(email, "@", "_"), email, passHash, role, status).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.user.create", "user", &id, map[string]any{"email": email, "role": role, "status": status})
	granted := 0
	extended := 0
	if len(req.ModeIDs) > 0 {
		grantReq := adminGrantAccessRequest{ModeIDs: req.ModeIDs, Days: req.Days}
		var grantErr error
		granted, extended, grantErr = h.grantAdminAccess(r, actor.ID, id, grantReq)
		if grantErr != nil {
			clearAdminExportOptionsCache()
			h.c.adminUsersList.clear()
			writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "userId": id, "accessError": grantErr.Error()})
			return
		}
	}
	clearAdminExportOptionsCache()
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "userId": id, "granted": granted, "extended": extended})
}
