package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GET /api/profile/export: everything the service holds about the caller, as
// one machine-readable JSON file (GDPR art. 15/20, 152-FZ art. 14).
//
// The target is always the session owner. There is no user id parameter on
// purpose: any "userId" in the query is ignored, so the endpoint cannot be
// turned into a way to read someone else's data.
//
// Payment method tokens are left out: they are identifiers issued by the
// payment provider for charging, not data about the person, and a file that
// leaves the server is the wrong place for them.

const userExportFormat = "mindstrata.user-export.v1"

type exportMessage struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

type exportDialog struct {
	ID        int64           `json:"id"`
	ModeID    *int64          `json:"modeId"`
	CreatedAt time.Time       `json:"createdAt"`
	DeletedAt *time.Time      `json:"deletedAt"`
	Messages  []exportMessage `json:"messages"`
}

func (h Handler) ProfileExport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	if !h.allowAuthAttempt(r, "profile_export", 10, time.Hour) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many requests"})
		return
	}
	payload, err := h.buildUserExport(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="mindstrata-export-%d.json"`, user.ID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h Handler) buildUserExport(ctx context.Context, userID int64) (map[string]any, error) {
	profile := map[string]any{}
	var (
		email, displayName, phone, role, status, telegramUsername *string
		createdAt                                                 time.Time
		lastLoginAt, emailVerifiedAt                              *time.Time
		allowAnonymization                                        bool
	)
	if err := h.DB.QueryRow(ctx, `
		select email, display_name, phone, role, status, telegram_username,
		       created_at, last_login_at, email_verified_at, allow_message_anonymization
		from users where id = $1`, userID).Scan(
		&email, &displayName, &phone, &role, &status, &telegramUsername,
		&createdAt, &lastLoginAt, &emailVerifiedAt, &allowAnonymization); err != nil {
		return nil, err
	}
	profile["id"] = userID
	profile["email"] = email
	profile["displayName"] = displayName
	profile["phone"] = phone
	profile["role"] = role
	profile["status"] = status
	profile["telegramUsername"] = telegramUsername
	profile["createdAt"] = createdAt
	profile["lastLoginAt"] = lastLoginAt
	profile["emailVerifiedAt"] = emailVerifiedAt
	profile["allowMessageAnonymization"] = allowAnonymization

	dialogs, err := h.exportDialogs(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"format":     userExportFormat,
		"exportedAt": time.Now().UTC(),
		"user":       profile,
		"dialogs":    dialogs,
	}
	// Optional sections: a table that does not exist yet (migration not
	// applied) must not break the export of everything else.
	if consents, err := h.listConsentRecords(ctx, userID); err == nil {
		out["consents"] = consents
	}
	out["cookieConsents"] = h.exportJSONRows(ctx, `
		select coalesce(json_agg(json_build_object(
			'consentId', consent_id, 'grantedAt', granted_at, 'sourcePath', source_path) order by granted_at), '[]'::json)
		from cookie_consents where user_id = $1`, userID)
	out["notificationContacts"] = h.exportJSONRows(ctx, `
		select coalesce(json_agg(json_build_object(
			'channel', channel, 'address', address, 'verified', verified, 'createdAt', created_at) order by id), '[]'::json)
		from notification_contacts where user_id = $1`, userID)
	out["subscriptions"] = h.exportJSONRows(ctx, `
		select coalesce(json_agg(json_build_object(
			'tariffId', tariff_id, 'status', status, 'activeFrom', active_from, 'activeTo', active_to,
			'amountPaid', amount_paid, 'createdAt', created_at) order by id), '[]'::json)
		from subscriptions where user_id = $1`, userID)
	out["invoices"] = h.exportJSONRows(ctx, `
		select coalesce(json_agg(json_build_object(
			'amount', amount, 'currency', currency, 'status', status, 'paidAt', paid_at,
			'createdAt', created_at) order by id), '[]'::json)
		from invoices where user_id = $1`, userID)
	out["files"] = h.exportJSONRows(ctx, `
		select coalesce(json_agg(json_build_object(
			'id', id, 'originalFilename', original_filename, 'mimeType', mime_type, 'createdAt', created_at) order by id), '[]'::json)
		from chat_files where owner_user_id = $1`, userID)
	return out, nil
}

func (h Handler) exportDialogs(ctx context.Context, userID int64) ([]exportDialog, error) {
	rows, err := h.DB.Query(ctx, `
		select id, mode_id, created_at, deleted_at
		from users_dialogs where user_id = $1
		order by id`, userID)
	if err != nil {
		return nil, err
	}
	dialogs := []exportDialog{}
	index := map[int64]int{}
	for rows.Next() {
		var d exportDialog
		if err := rows.Scan(&d.ID, &d.ModeID, &d.CreatedAt, &d.DeletedAt); err != nil {
			rows.Close()
			return nil, err
		}
		d.Messages = []exportMessage{}
		index[d.ID] = len(dialogs)
		dialogs = append(dialogs, d)
	}
	rows.Close()
	if len(dialogs) == 0 {
		return dialogs, nil
	}
	msgRows, err := h.DB.Query(ctx, `
		select m.dialog_id, m.id, m.role, m.content, m.created_at
		from dialogs_messages m
		join users_dialogs d on d.id = m.dialog_id
		where d.user_id = $1
		order by m.dialog_id, m.id`, userID)
	if err != nil {
		return nil, err
	}
	defer msgRows.Close()
	for msgRows.Next() {
		var dialogID int64
		var m exportMessage
		if err := msgRows.Scan(&dialogID, &m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		if i, ok := index[dialogID]; ok {
			dialogs[i].Messages = append(dialogs[i].Messages, m)
		}
	}
	return dialogs, msgRows.Err()
}

// exportJSONRows runs a query that returns one JSON array; on any error the
// section is exported as an empty list rather than failing the whole file.
func (h Handler) exportJSONRows(ctx context.Context, query string, userID int64) json.RawMessage {
	var raw []byte
	if err := h.DB.QueryRow(ctx, query, userID).Scan(&raw); err != nil || len(raw) == 0 {
		return json.RawMessage(`[]`)
	}
	return json.RawMessage(raw)
}
