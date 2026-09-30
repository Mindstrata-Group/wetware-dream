package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const adminMessagesTotalTTL = 60 * time.Second

// adminMessagesTotal is a Handler method that returns count(*) from dialogs_messages.
// Cached in h.c.adminMessagesTotal (TTL 60s) to avoid a 600ms full scan.
func (h Handler) adminMessagesTotal(ctx context.Context) int64 {
	c := &h.c.adminMessagesTotal
	c.Lock()
	if time.Now().Before(c.expiresAt) {
		v := c.value
		c.Unlock()
		return v
	}
	c.Unlock()
	var total int64
	if err := h.DB.QueryRow(ctx, `select count(*) from dialogs_messages`).Scan(&total); err != nil {
		return 0
	}
	c.Lock()
	c.value = total
	c.expiresAt = time.Now().Add(adminMessagesTotalTTL)
	c.Unlock()
	return total
}

type authLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authForgotPasswordRequest struct {
	Email string `json:"email"`
}

type authResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

func (h Handler) AuthLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !h.allowAuthAttempt(r, "login", 10, 10*time.Minute) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many attempts"})
		return
	}

	var req authLoginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := decodeJSONStrictBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" || strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "email and password are required"})
		return
	}

	var user AuthenticatedUser
	var passwordHash string
	err := h.DB.QueryRow(r.Context(), `
		select id, coalesce(email, ''), role, status, coalesce(password_hash, '')
		from users
		where lower(email) = lower($1)
		  and deleted_at is null`, email).Scan(&user.ID, &user.Email, &user.Role, &user.Status, &passwordHash)
	// S-NEW-3: always run PBKDF2, even for non-existent emails.
	// Otherwise the timings (fast path without a hash vs slow path with PBKDF2) reveal
	// whether the email exists in the database -> enumeration.
	var pwdOK bool
	if err != nil || passwordHash == "" {
		_ = dummyPasswordVerify(req.Password)
		pwdOK = false
	} else {
		pwdOK = verifyPassword(req.Password, passwordHash)
	}
	if err != nil || user.Status != "active" || passwordHash == "" || !pwdOK {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid credentials"})
		return
	}
	if err := h.createAuthSession(r.Context(), w, r, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if guestUserID := guestUserIDFromRequest(r, h.GuestCookieSecret); guestUserID > 0 {
		if err := h.transferGuestAccess(r.Context(), guestUserID, user.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		clearGuestCookie(w, r)
	}
	_, _ = h.DB.Exec(r.Context(), `update users set last_login_at = now() where id = $1`, user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
}

func (h Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	h.revokeAuthSession(r.Context(), r)
	clearSessionCookie(w, r)
	clearGuestCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func clearGuestCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     guestCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h Handler) AuthMe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
}

func (h Handler) AuthForgotPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !h.allowAuthAttempt(r, "forgot", 6, 30*time.Minute) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many attempts"})
		return
	}
	var req authForgotPasswordRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	_ = json.NewDecoder(r.Body).Decode(&req)
	email := normalizeEmail(req.Email)

	// S-NEW3-4: timing equalisation: regardless of whether the email exists
	// we do the same work: randomToken + tokenHash + DB write.
	// For a legit user, INSERT into password_reset_tokens.
	// For a non-existent one, an UPDATE by hash that finds nothing but
	// costs one DB round trip with the same index lookup cost as the INSERT.
	var devToken string
	token, tokenErr := randomToken(32)
	hash := ""
	if tokenErr == nil {
		hash = tokenHash(token)
	}
	if email != "" && tokenErr == nil {
		var userID int64
		err := h.DB.QueryRow(r.Context(), `select id from users where lower(email) = lower($1) and status = 'active' and deleted_at is null`, email).Scan(&userID)
		if err == nil {
			_, _ = h.DB.Exec(r.Context(), `
				insert into password_reset_tokens (user_id, token_hash, expires_at)
				values ($1, $2, $3)`, userID, hash, time.Now().Add(passwordResetTTL))
			devToken = token
		} else {
			// User not found: a symmetric DB write, an UPDATE by a token_hash
			// that does not exist. One index lookup + 0 affected rows.
			// CPU/IO profile ~= INSERT, but without a real side effect.
			_, _ = h.DB.Exec(r.Context(), `
				update password_reset_tokens set used_at = now()
				where token_hash = $1 and used_at is not null`, hash)
		}
	}

	resp := map[string]any{"ok": true, "message": "If the account exists, reset instructions will be sent."}
	if h.AuthDevReturnResetToken && devToken != "" {
		resp["resetToken"] = devToken
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h Handler) AuthResetPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !h.allowAuthAttempt(r, "reset", 8, 30*time.Minute) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many attempts"})
		return
	}
	var req authResetPasswordRequest
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := decodeJSONStrictBody(r, &req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	newHash, err := hashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback(r.Context())

	var userID int64
	err = tx.QueryRow(r.Context(), `
		select user_id
		from password_reset_tokens
		where token_hash = $1
		  and used_at is null
		  and expires_at > now()
		for update`, tokenHash(req.Token)).Scan(&userID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid or expired token"})
		return
	}
	if _, err = tx.Exec(r.Context(), `update users set password_hash = $2, updated_at = now() where id = $1`, userID, newHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(r.Context(), `update password_reset_tokens set used_at = now() where token_hash = $1`, tokenHash(req.Token)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(r.Context(), `update auth_sessions set revoked_at = now() where user_id = $1 and revoked_at is null`, userID)
	if err = tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type profileSettingsRequest struct {
	AllowMessageAnonymization *bool `json:"allowMessageAnonymization"`
}

func (h Handler) Profile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodDelete {
		h.deleteAccount(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	allowMessageAnonymization, err := h.allowMessageAnonymization(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	modes, err := h.getActiveAccessModes(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	stats, err := h.profileStats(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if user.Role == "admin" {
		stats["messages"] = h.adminMessagesTotal(r.Context())
	}
	billing, err := h.profileBillingSummary(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	notificationPrefs, err := h.notificationPreferencesPayload(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var maxChatID, telegramID *int64
	var maxBonusGranted bool
	_ = h.DB.QueryRow(r.Context(), `SELECT max_chat_id, max_bonus_granted, telegram_id FROM users WHERE id = $1 AND deleted_at IS NULL`, user.ID).
		Scan(&maxChatID, &maxBonusGranted, &telegramID)
	// Multi-binding: one Max chat can be bound to several accounts;
	// we show which others (masked) so the user understands the picture.
	maxAlsoLinkedTo := []string{}
	if maxChatID != nil {
		rows, err := h.DB.Query(r.Context(), `
			SELECT COALESCE(NULLIF(email, ''), 'аккаунт #' || id::text)
			FROM users WHERE max_chat_id = $1 AND id <> $2 AND deleted_at IS NULL
			ORDER BY id LIMIT 5`, *maxChatID, user.ID)
		if err == nil {
			for rows.Next() {
				var label string
				if rows.Scan(&label) == nil {
					maxAlsoLinkedTo = append(maxAlsoLinkedTo, maskEmailLabel(label))
				}
			}
			rows.Close()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                        true,
		"user":                      user,
		"hasAccess":                 len(modes) > 0,
		"activeModes":               modes,
		"activeTo":                  maxActiveTo(modes),
		"stats":                     stats,
		"billing":                   billing,
		"allowMessageAnonymization": allowMessageAnonymization,
		"notifications":             notificationPrefs,
		"maxLinked":                 maxChatID != nil,
		"maxBonusGranted":           maxBonusGranted,
		"maxBotLink":                h.maxBotStartLink(user.ID),
		"maxAlsoLinkedTo":           maxAlsoLinkedTo,
		"telegramLinked":            telegramID != nil,
		"telegramBotLink":           h.tgBotStartLink(user.ID),
	})
}

func (h Handler) ProfileSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}

	var req profileSettingsRequest
	if err := decodeJSONStrict(w, r, 8<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if req.AllowMessageAnonymization == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "allowMessageAnonymization is required"})
		return
	}

	var allow bool
	if err := h.DB.QueryRow(r.Context(), `
		UPDATE users
		SET allow_message_anonymization = $2, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING allow_message_anonymization`, user.ID, *req.AllowMessageAnonymization).Scan(&allow); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "allowMessageAnonymization": allow})
}

func (h Handler) allowMessageAnonymization(ctx context.Context, userID int64) (bool, error) {
	var allow bool
	err := h.DB.QueryRow(ctx, `
		SELECT allow_message_anonymization
		FROM users
		WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&allow)
	return allow, err
}

func (h Handler) profileStats(ctx context.Context, userID int64) (map[string]any, error) {
	var dialogs, messages, liveMessages, tokens int64
	err := h.DB.QueryRow(ctx, `
		with
		  d_m as (
		    select count(*) as dialog_n, coalesce(sum(message_count), 0) as message_n
		    from users_dialogs
		    where user_id = $1 and deleted_at is null
		  ),
		  u as (
		    select count(*) as live_n, coalesce(sum(total_tokens), 0) as token_n
		    from message_usage
		    where user_id = $1
		  )
		select
		  (select dialog_n from d_m),
		  (select message_n from d_m),
		  (select live_n from u),
		  (select token_n from u)
	`, userID).Scan(&dialogs, &messages, &liveMessages, &tokens)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return map[string]any{
		"dialogs":      dialogs,
		"messages":     messages,
		"liveMessages": liveMessages,
		"tokens":       tokens,
	}, nil
}

// deleteAccount: DELETE /api/profile deletes the current user's account.
// Anonymises PII, marks the user deleted, revokes all sessions.
// Financial records (invoices, subscriptions) are kept for accounting.
func (h Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	tx, err := beginTxTimeout(r.Context(), h.DB, dbAcquireTimeout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	// Remove access data and dialogs (soft delete)
	if _, err = tx.Exec(r.Context(), `delete from user_mode_access where user_id = $1`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(r.Context(),
		`update users_dialogs set deleted_at = now() where user_id = $1 and deleted_at is null`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(r.Context(), `delete from user_identities where user_id = $1`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Right to erasure (GDPR art. 17, 152-FZ art. 21): soft-deleted dialogs
	// alone left every message text in the database. The rows stay because
	// usage and billing records reference them; the content goes.
	// Spec: docs/specs/data-protection.md, AC-10.
	for _, stmt := range []string{
		`update dialogs_messages set content = '', anonymized_at = coalesce(anonymized_at, now())
		 where dialog_id in (select id from users_dialogs where user_id = $1)`,
		`delete from orchestration_decision_logs where user_id = $1`,
		`delete from chat_message_attachments where file_id in (select id from chat_files where owner_user_id = $1)`,
		`with removed as (delete from chat_files where owner_user_id = $1 returning blob_id)
		 delete from chat_file_blobs b
		 where b.id in (select blob_id from removed)
		   and not exists (select 1 from chat_files f where f.blob_id = b.id and f.owner_user_id <> $1)`,
		`delete from notification_contacts where user_id = $1`,
		`delete from notification_channel_consents where user_id = $1`,
		`delete from notification_reachability where user_id = $1`,
		`delete from notification_inbox where user_id = $1`,
	} {
		if _, err = tx.Exec(r.Context(), stmt, user.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	// Consent records are kept as proof but stop being active. The table may
	// not exist before the migration is applied: a savepoint keeps that from
	// aborting the whole deletion.
	if sp, spErr := tx.Begin(r.Context()); spErr == nil {
		if _, e := sp.Exec(r.Context(), `update consent_records set withdrawn_at = now() where user_id = $1 and withdrawn_at is null`, user.ID); e != nil {
			_ = sp.Rollback(r.Context())
		} else {
			_ = sp.Commit(r.Context())
		}
	}
	// Anonymise PII. status='blocked' (the only option besides active in the CHECK);
	// deleted_at IS NOT NULL distinguishes "deleted by the user" from admin-blocked.
	if _, err = tx.Exec(r.Context(), `
		update users set
			email = null, display_name = null, avatar_url = null, phone = null,
			password_hash = null, telegram_id = null, telegram_username = null,
			max_chat_id = null, max_linked_at = null, telegram_linked_at = null,
			status = 'blocked', deleted_at = now(), updated_at = now()
		where id = $1`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Revoke all sessions
	if _, err = tx.Exec(r.Context(),
		`update auth_sessions set revoked_at = now() where user_id = $1 and revoked_at is null`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
