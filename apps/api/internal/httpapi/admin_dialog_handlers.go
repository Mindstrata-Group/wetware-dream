package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const adminDialogsCacheTTL = 30 * time.Second

func (h Handler) AdminDialogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdminSection(w, r, "dialogs", false); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	h.c.adminDialogs.Lock()
	if h.c.adminDialogs.body != nil && time.Now().Before(h.c.adminDialogs.expiresAt) {
		body := h.c.adminDialogs.body
		h.c.adminDialogs.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	h.c.adminDialogs.Unlock()
	rows, err := h.DB.Query(r.Context(), `
        select d.id, d.user_id, d.mode_id, m.name, d.created_at,
               count(dm.id)::int as message_count,
               max(dm.created_at) as last_message_at
        from users_dialogs d
        join modes m on m.id = d.mode_id
        left join dialogs_messages dm on dm.dialog_id = d.id
        where d.deleted_at is null
        group by d.id, d.user_id, d.mode_id, m.name, d.created_at
        order by d.id desc
        limit 100`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out []DialogSummary
	for rows.Next() {
		var item DialogSummary
		if err := rows.Scan(&item.DialogID, &item.UserID, &item.ModeID, &item.ModeName, &item.CreatedAt, &item.MessageCount, &item.LastMessageAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, item)
	}
	body, _ := json.Marshal(map[string]any{"ok": true, "dialogs": out})
	h.c.adminDialogs.Lock()
	h.c.adminDialogs.body = body
	h.c.adminDialogs.expiresAt = time.Now().Add(adminDialogsCacheTTL)
	h.c.adminDialogs.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (h Handler) AdminDialogDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "dialogs", false)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/dialogs/")
	dialogID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || dialogID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid dialog id"})
		return
	}
	ctx := r.Context()
	var item DialogSummary
	err = h.DB.QueryRow(ctx, `
        select d.id, d.user_id, d.mode_id, m.name, d.created_at,
               count(dm.id)::int as message_count,
               max(dm.created_at) as last_message_at
        from users_dialogs d
        join modes m on m.id = d.mode_id
        left join dialogs_messages dm on dm.dialog_id = d.id
        where d.id = $1
        group by d.id, d.user_id, d.mode_id, m.name, d.created_at`, dialogID).Scan(&item.DialogID, &item.UserID, &item.ModeID, &item.ModeName, &item.CreatedAt, &item.MessageCount, &item.LastMessageAt)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": "dialog not found"})
		return
	}
	item.Messages, err = getDialogMessages(ctx, h.DB, dialogID, 200, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Personal-data access log: who opened whose conversation. Only ids go
	// into the record, never the text. Spec: data-protection AC-13.
	h.writeAdminAudit(ctx, r, actor.ID, "admin.dialog.read", "dialog", &dialogID, map[string]any{"subjectUserId": item.UserID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dialog": item})
}
