package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName = "mindstrata_session"
	authSessionTTL    = 30 * 24 * time.Hour
)

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h Handler) createAuthSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID int64) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	expires := time.Now().Add(authSessionTTL)
	_, err = h.DB.Exec(ctx, `
		insert into auth_sessions (user_id, token_hash, user_agent, ip_hash, expires_at)
		values ($1, $2, $3, $4, $5)`,
		userID,
		tokenHash(token),
		strings.TrimSpace(r.UserAgent()),
		requestIPHash(r),
		expires,
	)
	if err != nil {
		return err
	}
	setSessionCookie(w, r, token, expires)
	return nil
}

func (h Handler) revokeAuthSession(ctx context.Context, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" || h.DB == nil {
		return
	}
	_, _ = h.DB.Exec(ctx, `update auth_sessions set revoked_at = now() where token_hash = $1 and revoked_at is null`, tokenHash(cookie.Value))
}

func requestHasSessionCookie(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && strings.TrimSpace(cookie.Value) != ""
}

func writeGuestAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errAuthRequired) {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "auth required"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
}

func (h Handler) currentUser(ctx context.Context, r *http.Request) (AuthenticatedUser, error) {
	if h.DB == nil {
		return AuthenticatedUser{}, errAuthRequired
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return AuthenticatedUser{}, errAuthRequired
	}
	var user AuthenticatedUser
	err = h.DB.QueryRow(ctx, `
		select u.id, coalesce(u.email, ''), u.role, u.status
		from auth_sessions s
		join users u on u.id = s.user_id
		where s.token_hash = $1
		  and s.revoked_at is null
		  and s.expires_at > now()
		  and u.deleted_at is null
		  and u.status = 'active'`, tokenHash(cookie.Value)).Scan(&user.ID, &user.Email, &user.Role, &user.Status)
	if err != nil {
		return AuthenticatedUser{}, errAuthRequired
	}
	return user, nil
}

type AuthenticatedUserFull struct {
	AuthenticatedUser
	CurrentModeID   *int64
	CurrentDialogID *int64
}

func (h Handler) currentUserFull(ctx context.Context, r *http.Request) (AuthenticatedUserFull, error) {
	if h.DB == nil {
		return AuthenticatedUserFull{}, errAuthRequired
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return AuthenticatedUserFull{}, errAuthRequired
	}
	var full AuthenticatedUserFull
	err = h.DB.QueryRow(ctx, `
		select u.id, coalesce(u.email, ''), u.role, u.status, u.current_mode, u.current_dialog
		from auth_sessions s
		join users u on u.id = s.user_id
		where s.token_hash = $1
		  and s.revoked_at is null
		  and s.expires_at > now()
		  and u.deleted_at is null
		  and u.status = 'active'`, tokenHash(cookie.Value)).Scan(
		&full.ID, &full.Email, &full.Role, &full.Status,
		&full.CurrentModeID, &full.CurrentDialogID)
	if err != nil {
		return AuthenticatedUserFull{}, errAuthRequired
	}
	return full, nil
}
