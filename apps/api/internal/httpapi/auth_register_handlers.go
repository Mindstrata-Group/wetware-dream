package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const emailVerificationTTL = 24 * time.Hour

type authRegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Phone    string `json:"phone"`
}

func userHandleFromEmail(email string) string {
	local := strings.Split(normalizeEmail(email), "@")[0]
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		out = "user"
	}
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func (h Handler) AuthRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !h.allowAuthAttempt(r, "register", 8, 30*time.Minute) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many attempts"})
		return
	}

	var req authRegisterRequest
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := decodeJSONStrictBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" || !strings.Contains(email, "@") || strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "email and password are required"})
		return
	}
	passwordHash, err := hashPassword(req.Password)
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

	var existingID int64
	err = tx.QueryRow(r.Context(), `
		select id
		from users
		where lower(email) = lower($1)
		  and deleted_at is null`, email).Scan(&existingID)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "email already registered"})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	username := fmt.Sprintf("webuser_%s_%d", userHandleFromEmail(email), time.Now().Unix())
	var userID int64
	err = tx.QueryRow(r.Context(), `
		insert into users (
			email,
			password_hash,
			phone,
			role,
			status,
			accepted_tos,
			telegram_username,
			display_name,
			updated_at
		)
		values ($1, $2, nullif($3, ''), 'user', 'active', true, $4, $5, now())
		returning id`, email, passwordHash, strings.TrimSpace(req.Phone), username, userHandleFromEmail(email)).Scan(&userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	verifyToken, err := createEmailVerificationToken(r.Context(), tx, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if err := h.createAuthSession(r.Context(), w, r, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if guestUserID := guestUserIDFromRequest(r, h.GuestCookieSecret); guestUserID > 0 {
		if err := h.transferGuestAccess(r.Context(), guestUserID, userID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		clearGuestCookie(w, r)
	}
	// SMTP sending removed on 2026-05-28. The verify token is returned in the response
	// below if AUTH_DEV_RETURN_VERIFY_TOKEN=true; otherwise an admin gets it from the DB.

	resp := map[string]any{"ok": true, "message": "registered", "user": AuthenticatedUser{ID: userID, Email: email, Role: "user", Status: "active"}}
	if h.AuthDevReturnVerifyToken {
		resp["verifyToken"] = verifyToken
	}
	writeJSON(w, http.StatusOK, resp)
}

func createEmailVerificationToken(ctx context.Context, q pgx.Tx, userID int64) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = q.Exec(ctx, `
		insert into email_verification_tokens (user_id, token_hash, expires_at)
		values ($1, $2, $3)`, userID, tokenHash(token), time.Now().Add(emailVerificationTTL))
	if err != nil {
		return "", err
	}
	return token, nil
}

func (h Handler) AuthVerifyEmail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" && r.Method == http.MethodPost {
		var payload struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&payload)
		token = strings.TrimSpace(payload.Token)
	}
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "token is required"})
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
		from email_verification_tokens
		where token_hash = $1
		  and used_at is null
		  and expires_at > now()
		for update`, tokenHash(token)).Scan(&userID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid or expired token"})
		return
	}
	if _, err = tx.Exec(r.Context(), `update users set email_verified_at = now(), updated_at = now() where id = $1`, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(r.Context(), `update email_verification_tokens set used_at = now() where token_hash = $1`, tokenHash(token)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if r.Method == http.MethodGet {
		http.Redirect(w, r, publicWebBaseURL()+"/profile?email=verified", http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
