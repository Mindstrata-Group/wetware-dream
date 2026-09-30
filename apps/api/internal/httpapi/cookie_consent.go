package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type cookieConsentRequest struct {
	ConsentID  string `json:"consentId"`
	SourcePath string `json:"sourcePath"`
}

func (h Handler) CookieConsent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	// K-NEW3-3: rate limit. Without it an attacker can INSERT into the DB without bound,
	// bloating cookie_consents to hundreds of MB. 10/min/IP is fine for a legit user (a CB changes
	// consent < 5 times/min), but not for a bot.
	if !h.allowAuthAttempt(r, "cookie_consent", 10, time.Minute) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many requests"})
		return
	}

	var req cookieConsentRequest
	if err := decodeJSONStrict(w, r, 4<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	consentID := strings.TrimSpace(req.ConsentID)
	if consentID == "" || len(consentID) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "consentId is required"})
		return
	}

	var userID *int64
	if user, err := h.currentUser(r.Context(), r); err == nil && user.ID > 0 {
		userID = &user.ID
	}

	sourcePath := strings.TrimSpace(req.SourcePath)
	if len(sourcePath) > 512 {
		sourcePath = sourcePath[:512]
	}
	userAgent := strings.TrimSpace(r.UserAgent())
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}

	_, err := h.DB.Exec(r.Context(), `
		insert into cookie_consents (consent_id, user_id, source_path, user_agent, ip_hash)
		values ($1, $2, $3, $4, $5)
		on conflict (consent_id) do update
		set user_id = coalesce(excluded.user_id, cookie_consents.user_id),
		    source_path = coalesce(nullif(excluded.source_path, ''), cookie_consents.source_path),
		    user_agent = coalesce(nullif(excluded.user_agent, ''), cookie_consents.user_agent),
		    granted_at = now()`,
		consentID, userID, sourcePath, userAgent, requestIPHash(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
