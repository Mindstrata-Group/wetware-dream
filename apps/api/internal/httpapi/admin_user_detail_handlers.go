package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (h Handler) AdminUserDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "users", r.Method != http.MethodGet)
	if !ok {
		return
	}
	userID, suffix, err := parseIDFromPath(r.URL.Path, "/api/admin/users/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid user id"})
		return
	}
	if suffix == "access" {
		h.adminUserAccess(w, r, actor, userID)
		return
	}
	switch r.Method {
	case http.MethodGet:
		// Personal-data access log (data-protection AC-13). The tester route
		// logs its own action before reusing adminGetUser.
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.user.read", "user", &userID, map[string]any{"subjectUserId": userID})
		h.adminGetUser(w, r, userID)
	case http.MethodPatch:
		h.adminPatchUser(w, r, actor, userID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) adminGetUser(w http.ResponseWriter, r *http.Request, userID int64) {
	var item map[string]any
	var id int64
	var email, role, status, username string
	var createdAt time.Time
	var lastLoginAt *time.Time
	err := h.DB.QueryRow(r.Context(), `
		select id, coalesce(email, ''), role, status, coalesce(telegram_username, ''), created_at, last_login_at
		from users
		where id = $1 and deleted_at is null`, userID).Scan(&id, &email, &role, &status, &username, &createdAt, &lastLoginAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "user not found"})
		return
	}
	access, _ := h.getAdminUserAccessModes(r.Context(), userID)
	stats, _ := h.profileStats(r.Context(), userID)
	item = map[string]any{"id": id, "email": email, "role": role, "status": status, "telegramUsername": username, "createdAt": createdAt, "lastLoginAt": lastLoginAt, "activeModes": access, "stats": stats}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": item})
}

func (h Handler) getAdminUserAccessModes(ctx context.Context, userID int64) ([]adminAccessModeItem, error) {
	rows, err := h.DB.Query(ctx, `
		select id, mode_id, name, active_from, active_to, daily_message_limit, priority, access_type, source_id, source_label
		from (
			select distinct on (uma.mode_id)
			       uma.id, uma.mode_id, m.name, uma.active_from, uma.active_to, uma.daily_message_limit, coalesce(uma.priority, 0) as priority, coalesce(uma.access_type, '') as access_type, uma.source_id,
			       case
			         when uma.access_type = 'promocode' then coalesce('промокод ' || p.code, 'промокод #' || uma.source_id::text, 'промокод')
			         when uma.access_type = 'subscription' then coalesce('подписка #' || s.id::text || ' · ' || t.name, 'подписка #' || uma.source_id::text, 'подписка')
			         when uma.access_type = 'manual' then coalesce('вручную админом ' || nullif(au.email, ''), 'вручную actor#' || uma.source_id::text, 'вручную')
			         else coalesce(uma.access_type, 'доступ')
			       end as source_label
			from user_mode_access uma
			join modes m on m.id = uma.mode_id
			left join promocodes p on uma.access_type = 'promocode' and p.id = uma.source_id
			left join subscriptions s on uma.access_type = 'subscription' and s.id = uma.source_id
			left join tariffs t on t.id = s.tariff_id
			left join users au on uma.access_type = 'manual' and au.id = uma.source_id
			where uma.user_id = $1
			  and (uma.active_from is null or uma.active_from <= now())
			  and (uma.active_to is null or uma.active_to >= now())
			order by uma.mode_id, coalesce(uma.priority, 0) desc, (uma.active_to is null) desc, uma.active_to desc nulls first, uma.id desc
		) active_access
		order by name asc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	soon := now.AddDate(0, 0, 7)
	out := []adminAccessModeItem{}
	for rows.Next() {
		var item adminAccessModeItem
		if err := rows.Scan(&item.ID, &item.ModeID, &item.ModeName, &item.ActiveFrom, &item.ActiveTo, &item.DailyMessageLimit, &item.Priority, &item.AccessType, &item.SourceID, &item.SourceLabel); err != nil {
			return nil, err
		}
		item.Status = "active"
		if item.ActiveTo != nil && item.ActiveTo.Before(now) {
			item.Status = "expired"
		} else if item.ActiveTo != nil && item.ActiveTo.Before(soon) {
			item.Status = "expiring"
		}
		if quota, err := h.dailyQuota(ctx, userID, item.ModeID); err == nil {
			item.Quota = quota
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h Handler) adminPatchUser(w http.ResponseWriter, r *http.Request, actor AuthenticatedUser, userID int64) {
	var req adminPatchUserRequest
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := decodeJSONStrictBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	var email, role, status, passwordHash string
	err := h.DB.QueryRow(r.Context(), `select coalesce(email, ''), role, status, coalesce(password_hash, '') from users where id = $1 and deleted_at is null`, userID).Scan(&email, &role, &status, &passwordHash)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "user not found"})
		return
	}
	if req.Email != nil {
		email = normalizeEmail(*req.Email)
	}
	if req.Role != nil {
		role = strings.TrimSpace(*req.Role)
	}
	if req.Status != nil {
		status = strings.TrimSpace(*req.Status)
	}
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		var hashErr error
		passwordHash, hashErr = hashPassword(*req.Password)
		if hashErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": hashErr.Error()})
			return
		}
	}
	if !validRole(role) || !validStatus(status) || email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid email, role or status"})
		return
	}
	// K-NEW-4: privilege escalation. Only an owner may assign the owner role OR
	// change an existing owner. role was already read earlier in this function (current state).
	if actor.Role != "owner" {
		if role == "owner" {
			writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "only owner can assign owner role"})
			return
		}
		// role in this function is the target's current role (see the SELECT above when loading the user).
		// The role variable is used before it is overwritten from req.Role.
		var currentRole string
		_ = h.DB.QueryRow(r.Context(), `select role from users where id = $1`, userID).Scan(&currentRole)
		if currentRole == "owner" {
			writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "only owner can modify owner account"})
			return
		}
	}
	if actor.ID == userID {
		if role != actor.Role {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "admin cannot change own role"})
			return
		}
		if status != "active" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "admin cannot block own account"})
			return
		}
	}
	_, err = h.DB.Exec(r.Context(), `
		update users
		set email = $2, role = $3, status = $4, password_hash = $5, updated_at = now()
		where id = $1`, userID, email, role, status, passwordHash)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		_, _ = h.DB.Exec(r.Context(), `update auth_sessions set revoked_at = now() where user_id = $1 and revoked_at is null`, userID)
	}
	clearAdminExportOptionsCache()
	h.c.adminUsersList.clear()
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.user.patch", "user", &userID, map[string]any{"role": role, "status": status})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
