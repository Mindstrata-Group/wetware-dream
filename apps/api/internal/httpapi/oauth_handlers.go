package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	oauthStateTTL        = 10 * time.Minute
	oauthStateCookieName = "mindstrata_oauth_state"
	oauthRegisterIntent  = "register"
	oauthStateIntentSep  = "\x1f"
)

type OAuthProfile struct {
	Provider       string
	ProviderUserID string
	Email          string
	EmailVerified  bool
	Phone          string
	PhoneVerified  bool
	DisplayName    string
	AvatarURL      string
	Raw            map[string]any
}

func (h Handler) AuthOAuthProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"providers": map[string]bool{
			"yandex": h.oauthProviderConfigured("yandex"),
		},
	})
}

func (h Handler) AuthOAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/auth/oauth/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
		return
	}
	provider, action := parts[0], parts[1]
	switch action {
	case "start":
		h.oauthStart(w, r, provider)
	case "callback":
		h.oauthCallback(w, r, provider)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
	}
}

func safeRedirectAfter(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/profile"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") {
		return "/profile"
	}
	if strings.HasPrefix(raw, "/admin") || strings.HasPrefix(raw, "/tester") || strings.HasPrefix(raw, "/expert") {
		return "/profile"
	}
	return raw
}

func oauthIntentFromRequest(r *http.Request) string {
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("intent")), oauthRegisterIntent) {
		return oauthRegisterIntent
	}
	return "login"
}

func packOAuthRedirectAfter(redirectAfter, intent string) string {
	redirectAfter = safeRedirectAfter(redirectAfter)
	if intent == oauthRegisterIntent {
		return oauthRegisterIntent + oauthStateIntentSep + redirectAfter
	}
	return redirectAfter
}

func unpackOAuthRedirectAfter(stored string) (string, string) {
	stored = strings.TrimSpace(stored)
	prefix := oauthRegisterIntent + oauthStateIntentSep
	if strings.HasPrefix(stored, prefix) {
		return safeRedirectAfter(strings.TrimPrefix(stored, prefix)), oauthRegisterIntent
	}
	return safeRedirectAfter(stored), "login"
}

func setOAuthStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/api/auth/oauth/",
		Expires:  time.Now().Add(oauthStateTTL),
		MaxAge:   int(oauthStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/api/auth/oauth/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h Handler) oauthStart(w http.ResponseWriter, r *http.Request, provider string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	cfg, err := h.oauthConfig(provider)
	if err != nil {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=oauth_not_configured", http.StatusFound)
		return
	}
	state, err := randomToken(32)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	redirectAfter := safeRedirectAfter(r.URL.Query().Get("next"))
	oauthIntent := oauthIntentFromRequest(r)
	_, err = h.DB.Exec(r.Context(), `
		insert into oauth_states (token_hash, provider, redirect_after, ip_hash, user_agent, expires_at)
		values ($1, $2, $3, $4, $5, $6)`,
		tokenHash(state),
		provider,
		packOAuthRedirectAfter(redirectAfter, oauthIntent),
		requestIPHash(r),
		strings.TrimSpace(r.UserAgent()),
		time.Now().Add(oauthStateTTL),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	setOAuthStateCookie(w, r, state)
	http.Redirect(w, r, cfg.AuthURL(state, r.URL.Query().Get("force_account") == "1"), http.StatusFound)
}

func (h Handler) oauthCallback(w http.ResponseWriter, r *http.Request, provider string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=oauth", http.StatusFound)
		return
	}
	if cookie, err := r.Cookie(oauthStateCookieName); err != nil || cookie.Value == "" || cookie.Value != state {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=oauth_state", http.StatusFound)
		return
	}
	clearOAuthStateCookie(w, r)

	redirectAfter, oauthIntent, err := h.consumeOAuthState(r.Context(), provider, state)
	if err != nil {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=oauth_expired", http.StatusFound)
		return
	}

	profile, err := h.exchangeOAuthProfile(r.Context(), provider, code)
	if err != nil {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=oauth_profile", http.StatusFound)
		return
	}

	guestUserID := guestUserIDFromRequest(r, h.GuestCookieSecret)
	var user AuthenticatedUser
	if oauthIntent == oauthRegisterIntent {
		user, err = h.findOrCreateOAuthUser(r.Context(), profile)
	} else {
		user, err = h.findOAuthUserForLogin(r.Context(), profile)
	}
	if err != nil {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=oauth_account", http.StatusFound)
		return
	}
	if err := h.transferGuestAccess(r.Context(), guestUserID, user.ID); err != nil {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=guest_access", http.StatusFound)
		return
	}
	clearGuestCookie(w, r)
	if err := h.createAuthSession(r.Context(), w, r, user.ID); err != nil {
		http.Redirect(w, r, publicWebBaseURL()+"/login?error=session", http.StatusFound)
		return
	}
	_, _ = h.DB.Exec(r.Context(), `update users set last_login_at = now() where id = $1`, user.ID)
	http.Redirect(w, r, publicWebBaseURL()+redirectAfter, http.StatusFound)
}

func (h Handler) consumeOAuthState(ctx context.Context, provider, state string) (string, string, error) {
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	var storedProvider, redirectAfter string
	err = tx.QueryRow(ctx, `
		select provider, redirect_after
		from oauth_states
		where token_hash = $1
		  and used_at is null
		  and expires_at > now()
		for update`, tokenHash(state)).Scan(&storedProvider, &redirectAfter)
	if err != nil {
		return "", "", err
	}
	if storedProvider != provider {
		return "", "", errors.New("provider mismatch")
	}
	if _, err = tx.Exec(ctx, `update oauth_states set used_at = now() where token_hash = $1`, tokenHash(state)); err != nil {
		return "", "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}
	redirect, intent := unpackOAuthRedirectAfter(redirectAfter)
	return redirect, intent, nil
}
