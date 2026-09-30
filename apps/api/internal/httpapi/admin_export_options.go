package httpapi

import (
	"context"
	"net/http"
	"time"
)

func (h Handler) adminExportOptions(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	adminExportOptionsCache.RLock()
	if now.Before(adminExportOptionsCache.expiresAt) && adminExportOptionsCache.users != nil && adminExportOptionsCache.modes != nil && adminExportOptionsCache.promocodes != nil {
		users := cloneMapRows(adminExportOptionsCache.users)
		modes := cloneMapRows(adminExportOptionsCache.modes)
		promocodes := cloneMapRows(adminExportOptionsCache.promocodes)
		adminExportOptionsCache.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cached": true, "users": users, "modes": modes, "promocodes": promocodes})
		return
	}
	adminExportOptionsCache.RUnlock()

	users, err := h.queryAdminExportUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	modes, err := h.queryAdminExportModes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	promocodes, err := h.queryAdminExportPromocodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	adminExportOptionsCache.Lock()
	adminExportOptionsCache.users = cloneMapRows(users)
	adminExportOptionsCache.modes = cloneMapRows(modes)
	adminExportOptionsCache.promocodes = cloneMapRows(promocodes)
	adminExportOptionsCache.expiresAt = time.Now().Add(adminExportOptionsCacheTTL)
	adminExportOptionsCache.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cached": false, "users": users, "modes": modes, "promocodes": promocodes})
}

func (h Handler) queryAdminExportUsers(ctx context.Context) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, `
		select u.id, coalesce(u.email, ''), u.role, u.status, coalesce(u.telegram_username, ''), u.created_at, u.last_login_at,
		       coalesce(count(dm.id), 0)
		from users u
		left join users_dialogs d on d.user_id = u.id
		left join dialogs_messages dm on dm.dialog_id = d.id
		where u.deleted_at is null
		group by u.id
		order by u.id asc
		limit 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, messageCount int64
		var email, role, status, username string
		var createdAt time.Time
		var lastLoginAt *time.Time
		if err := rows.Scan(&id, &email, &role, &status, &username, &createdAt, &lastLoginAt, &messageCount); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "email": email, "role": role, "status": status, "telegramUsername": username, "createdAt": createdAt, "lastLoginAt": lastLoginAt, "messageCount": messageCount})
	}
	return out, rows.Err()
}

func (h Handler) queryAdminExportModes(ctx context.Context) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, `
		select id, name, coalesce(welcome_message, ''), ai_model, model_temperature, hidden_at is not null,
		       exists(select 1 from tariff_mode tm join tariffs t on t.id = tm.tariff_id where tm.mode_id = modes.id and coalesce(t.monthly_price, 0) > 0 and t.available_for_subscription = true and t.archived_at is null),
		       coalesce((select string_agg(modes.name || ' → ' || t.name || coalesce(' → ' || tg.name, ''), '; ' order by t.name) from tariff_mode tm join tariffs t on t.id = tm.tariff_id left join tariff_groups tg on tg.id = t.group_id where tm.mode_id = modes.id and coalesce(t.monthly_price,0) > 0 and t.available_for_subscription = true and t.archived_at is null), '')
		from modes
		order by id asc
		limit 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, welcome, aiModel, paidChains string
		var temperature float64
		var hidden, paid bool
		if err := rows.Scan(&id, &name, &welcome, &aiModel, &temperature, &hidden, &paid, &paidChains); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "welcomeMessage": welcome, "aiModel": aiModel, "modelTemperature": temperature, "hidden": hidden, "paid": paid, "paidChains": paidChains})
	}
	return out, rows.Err()
}

func (h Handler) queryAdminExportPromocodes(ctx context.Context) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, `
		select p.id, p.code, p.grants_type, p.target_id, p.max_uses, p.used_count, p.active_from, p.active_to,
		       coalesce(p.comment, ''), coalesce(p.purpose, ''), p.summary_limit, coalesce(p.temporary_admin_enabled, false),
		       max(u.used_at)
		from promocodes p
		left join promocode_usages u on u.promocode_id = p.id
		group by p.id
		order by p.id asc
		limit 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, targetID int64
		var code, grantsType, comment, purpose string
		var maxUses, usedCount int
		var activeFrom, activeTo, lastUsedAt *time.Time
		var summaryLimit *int64
		var temporaryAdmin bool
		if err := rows.Scan(&id, &code, &grantsType, &targetID, &maxUses, &usedCount, &activeFrom, &activeTo, &comment, &purpose, &summaryLimit, &temporaryAdmin, &lastUsedAt); err != nil {
			return nil, err
		}
		targetIDs, err := h.promocodeTargetIDs(ctx, id, targetID)
		if err != nil {
			return nil, err
		}
		modeIDs, err := h.promocodeModeIDsForTargets(ctx, grantsType, targetIDs)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "code": code, "grantsType": grantsType, "targetId": targetID, "targetIds": targetIDs, "modeIds": modeIDs, "maxUses": maxUses, "usedCount": usedCount, "activeFrom": activeFrom, "activeTo": activeTo, "comment": comment, "purpose": purpose, "summaryLimit": summaryLimit, "temporaryAdmin": temporaryAdmin, "lastUsedAt": lastUsedAt, "active": promocodeIsActive(activeFrom, activeTo, maxUses, usedCount)})
	}
	return out, rows.Err()
}
