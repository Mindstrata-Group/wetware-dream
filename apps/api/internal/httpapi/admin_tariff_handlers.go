package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func (h Handler) AdminTariffs(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdminSection(w, r, "tariffs", r.Method != http.MethodGet)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		h.adminCreateTariff(w, r, user)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	sort := r.URL.Query().Get("sort")
	orderBy := "t.created_at desc, t.id desc"
	switch sort {
	case "price":
		orderBy = "t.monthly_price asc, t.id desc"
	case "group":
		orderBy = "coalesce(tg.name, '') asc, t.id desc"
	case "type":
		orderBy = "coalesce(t.tariff_type, '') asc, t.id desc"
	case "created":
		orderBy = "t.created_at desc, t.id desc"
	}
	where := "where t.archived_at is null"
	if r.URL.Query().Get("includeArchived") == "true" {
		where = ""
	}
	rows, err := h.DB.Query(r.Context(), fmt.Sprintf(`
		select t.id, t.name, coalesce(t.description, ''), coalesce(t.tariff_type, ''), coalesce(t.monthly_price, 0), t.daily_message_limit, coalesce(t.limit_type, ''), t.group_id, t.available_for_subscription,
		       t.created_at, t.archived_at, t.first_mode_id, coalesce(tg.name, ''), coalesce(tg.available_for_subscription, false)
		from tariffs t
		left join tariff_groups tg on tg.id = t.group_id
		%s
		order by %s`, where, orderBy))
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
		var dailyLimit *int64
		var groupID *int64
		var firstModeID *int64
		var available, groupAvailable bool
		var createdAt time.Time
		var archivedAt *time.Time
		if err := rows.Scan(&id, &name, &description, &tariffType, &monthlyPrice, &dailyLimit, &limitType, &groupID, &available, &createdAt, &archivedAt, &firstModeID, &groupName, &groupAvailable); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		modeIDs, _ := h.queryTariffModeIDs(r.Context(), id)
		out = append(out, map[string]any{"id": id, "name": name, "description": description, "tariffType": tariffType, "monthlyPrice": monthlyPrice, "dailyMessageLimit": dailyLimit, "limitType": limitType, "groupId": groupID, "availableForSubscription": available, "createdAt": createdAt, "archivedAt": archivedAt, "groupName": groupName, "groupAvailableForSubscription": groupAvailable, "modeIds": modeIDs, "firstModeId": firstModeID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tariffs": out})
}

func notFoundIfNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return err
}
