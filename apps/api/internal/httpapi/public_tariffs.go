package httpapi

import (
	"context"
	"net/http"
	"time"
)

// PublicTariffs: GET /api/public/tariffs, the public storefront of active tariffs.
// We return only what the shop needs: prices, limits and mode names.
func (h Handler) PublicTariffs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		select
			t.id,
			t.name,
			coalesce(t.description, ''),
			coalesce(t.tariff_type, ''),
			coalesce(t.monthly_price, 0),
			t.daily_message_limit,
			coalesce(t.limit_type, ''),
			coalesce(tg.name, ''),
			t.created_at
		from tariffs t
		left join tariff_groups tg on tg.id = t.group_id
		where t.archived_at is null
		  and t.available_for_subscription = true
		  and t.tariff_type = 'regular'
		  and t.monthly_price > 0
		  and t.name not ilike 'AUTOPROMOCODETARIFF%'
		  and (tg.id is null or tg.available_for_subscription = true)
		order by coalesce(tg.sort_order, 999999) asc, t.monthly_price asc, t.id asc`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, description, tariffType, limitType, groupName string
		var monthlyPrice float64
		var dailyLimit int64
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &description, &tariffType, &monthlyPrice, &dailyLimit, &limitType, &groupName, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		modes, modeIDs, err := h.queryPublicTariffModes(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, map[string]any{
			"id":                       id,
			"name":                     name,
			"description":              description,
			"tariffType":               tariffType,
			"monthlyPrice":             monthlyPrice,
			"dailyMessageLimit":        dailyLimit,
			"limitType":                limitType,
			"groupName":                groupName,
			"modeIds":                  modeIDs,
			"modes":                    modes,
			"createdAt":                createdAt,
			"availableForSubscription": true,
		})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tariffs": out})
}

func (h Handler) queryPublicTariffModes(ctx context.Context, tariffID int64) ([]map[string]any, []int64, error) {
	rows, err := h.DB.Query(ctx, `
		select m.id, m.name, coalesce(m.welcome_message, '')
		from tariff_mode tm
		join modes m on m.id = tm.mode_id
		where tm.tariff_id = $1
		  and m.hidden_at is null
		order by m.id asc`, tariffID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	modes := []map[string]any{}
	ids := []int64{}
	for rows.Next() {
		var id int64
		var name, welcomeMessage string
		if err := rows.Scan(&id, &name, &welcomeMessage); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		modes = append(modes, map[string]any{"id": id, "name": name, "welcomeMessage": welcomeMessage})
	}
	return modes, ids, rows.Err()
}
