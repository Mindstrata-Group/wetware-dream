package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Consent journal: /api/privacy/consents (GET list, POST grant) and
// /api/privacy/consents/withdraw (POST). Spec: docs/specs/data-protection.md.
//
// The client sends the version and SHA-256 of the exact text it displayed;
// the journal stores both, so "which wording did this person accept" has an
// answer for every act of consent. Verifying the hash against a canonical
// text on the server is spec-only for now (AC-19).

type consentRecordView struct {
	ID          int64      `json:"id"`
	Document    string     `json:"document"`
	Version     string     `json:"version"`
	TextSHA256  string     `json:"textSha256"`
	GrantedAt   time.Time  `json:"grantedAt"`
	WithdrawnAt *time.Time `json:"withdrawnAt"`
}

// privacySubject resolves who is asking: a logged-in user, or a guest with a
// valid signed guest cookie (guests chat too and may need to consent).
// A present but invalid session cookie is an error and must clear the
// cookie; guest fallback applies only when no session cookie is sent.
func (h Handler) privacySubject(ctx context.Context, r *http.Request) (int64, error) {
	if user, err := h.currentUser(ctx, r); err == nil && user.ID > 0 {
		return user.ID, nil
	}
	if requestHasSessionCookie(r) {
		return 0, errAuthRequired
	}
	guestID := guestUserIDFromRequest(r, h.GuestCookieSecret)
	if guestID <= 0 || h.DB == nil {
		return 0, errAuthRequired
	}
	var id int64
	if err := h.DB.QueryRow(ctx, `select id from users where id = $1 and deleted_at is null`, guestID).Scan(&id); err != nil {
		return 0, errAuthRequired
	}
	return id, nil
}

func (h Handler) PrivacyConsents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	userID, err := h.privacySubject(r.Context(), r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := h.listConsentRecords(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "consent journal is unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "consents": items})
	case http.MethodPost:
		if !h.allowAuthAttempt(r, "privacy_consent", 30, time.Minute) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many requests"})
			return
		}
		var req struct {
			Document   string `json:"document"`
			Version    string `json:"version"`
			TextSHA256 string `json:"textSha256"`
		}
		if err := decodeJSONStrict(w, r, 4<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		document := strings.TrimSpace(req.Document)
		version := strings.TrimSpace(req.Version)
		hash := strings.TrimSpace(req.TextSHA256)
		if err := validateConsentPayload(document, version, hash); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		var id int64
		if err := h.DB.QueryRow(r.Context(), `
			insert into consent_records (user_id, document, version, text_sha256, ip_hash)
			values ($1, $2, $3, $4, $5)
			returning id`, userID, document, version, hash, requestIPHash(r)).Scan(&id); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "consent journal is unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) PrivacyConsentsWithdraw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	userID, err := h.privacySubject(r.Context(), r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}
	var req struct {
		Document string `json:"document"`
	}
	if err := decodeJSONStrict(w, r, 1<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	document := strings.TrimSpace(req.Document)
	if !consentDocuments[document] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "unknown consent document"})
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
		update consent_records set withdrawn_at = now()
		where user_id = $1 and document = $2 and withdrawn_at is null`, userID, document)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "consent journal is unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "withdrawn": tag.RowsAffected()})
}

func (h Handler) listConsentRecords(ctx context.Context, userID int64) ([]consentRecordView, error) {
	rows, err := h.DB.Query(ctx, `
		select id, document, version, text_sha256, granted_at, withdrawn_at
		from consent_records
		where user_id = $1
		order by granted_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []consentRecordView{}
	for rows.Next() {
		var item consentRecordView
		if err := rows.Scan(&item.ID, &item.Document, &item.Version, &item.TextSHA256, &item.GrantedAt, &item.WithdrawnAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return items, nil
}
