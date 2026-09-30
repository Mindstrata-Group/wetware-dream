//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestAuthFlow_ForgotPassword_ReturnsTokenInDevMode:
// AUTH_DEV_RETURN_RESET_TOKEN=true → forgot-password returns resetToken in JSON.
func TestAuthFlow_ForgotPassword_ReturnsTokenInDevMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{AuthDevReturnResetToken: true})

	user := f.CreateUser(TestUserOpts{})

	code, body := httpJSON(t, ts, "POST", "/api/auth/forgot-password", map[string]any{
		"email": user.Email,
	})
	if code != http.StatusOK {
		t.Fatalf("forgot: %d body=%v", code, body)
	}
	token, _ := body["resetToken"].(string)
	if token == "" {
		t.Fatalf("expected resetToken in dev mode, body=%v", body)
	}

	// DB: token stored with the correct userID
	var dbUserID int64
	_ = env.Pool.QueryRow(context.Background(),
		`select user_id from password_reset_tokens where token_hash = $1`,
		tokenHash(token)).Scan(&dbUserID)
	if dbUserID != user.ID {
		t.Fatalf("token's user_id: got %d want %d", dbUserID, user.ID)
	}
}

// TestAuthFlow_ForgotPassword_NoUserEnumeration:
// a request with a non-existent email → 200 ok without resetToken (we do not disclose).
func TestAuthFlow_ForgotPassword_NoUserEnumeration(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{AuthDevReturnResetToken: true})

	code, body := httpJSON(t, ts, "POST", "/api/auth/forgot-password", map[string]any{
		"email": "ghost@test.local",
	})
	if code != http.StatusOK {
		t.Fatalf("expected 200 for ghost email (no enumeration), got %d", code)
	}
	if token, _ := body["resetToken"].(string); token != "" {
		t.Fatalf("resetToken leaked for nonexistent email: %s", token)
	}
}

// TestAuthFlow_ResetPassword_FullCycle:
// forgot → token → reset → login with new password → ok.
// Also: the old session must be revoked.
func TestAuthFlow_ResetPassword_FullCycle(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{AuthDevReturnResetToken: true})

	user := f.CreateUser(TestUserOpts{Password: "oldpass123456"})

	// Old session created and valid
	oldToken := f.CreateSession(user.ID)
	ts.LoginAs(oldToken)
	code, _ := httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("pre-reset /me: %d", code)
	}

	// 1. Forgot password → get resetToken
	code, body := httpJSON(t, ts, "POST", "/api/auth/forgot-password", map[string]any{
		"email": user.Email,
	})
	if code != http.StatusOK {
		t.Fatalf("forgot: %d", code)
	}
	resetToken, _ := body["resetToken"].(string)
	if resetToken == "" {
		t.Fatalf("no resetToken in response: %v", body)
	}

	// 2. Reset password
	code, _ = httpJSON(t, ts, "POST", "/api/auth/reset-password", map[string]any{
		"token":       resetToken,
		"newPassword": "newpass987654",
	})
	if code != http.StatusOK {
		t.Fatalf("reset: %d", code)
	}

	// 3. The old session must be invalidated
	ts.LoginAs(oldToken)
	code, _ = httpJSON(t, ts, "GET", "/api/auth/me", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("old session after reset: expected 401, got %d", code)
	}

	// 4. Login with the old password → 401
	ts2 := NewTestServer(t, env.Pool)
	code, _ = httpJSON(t, ts2, "POST", "/api/auth/login", map[string]any{
		"email": user.Email, "password": "oldpass123456",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("login with old password: expected 401, got %d", code)
	}

	// 5. Login with the new password → 200
	ts3 := NewTestServer(t, env.Pool)
	code, _ = httpJSON(t, ts3, "POST", "/api/auth/login", map[string]any{
		"email": user.Email, "password": "newpass987654",
	})
	if code != http.StatusOK {
		t.Fatalf("login with new password: %d", code)
	}
}

// TestAuthFlow_ResetPassword_TokenReused: one token, one use.
func TestAuthFlow_ResetPassword_TokenReused(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{AuthDevReturnResetToken: true})

	user := f.CreateUser(TestUserOpts{})
	_, body := httpJSON(t, ts, "POST", "/api/auth/forgot-password", map[string]any{
		"email": user.Email,
	})
	token, _ := body["resetToken"].(string)

	// First reset: ok
	code, _ := httpJSON(t, ts, "POST", "/api/auth/reset-password", map[string]any{
		"token": token, "newPassword": "newpass987654",
	})
	if code != http.StatusOK {
		t.Fatalf("first reset: %d", code)
	}

	// Second with the same token: fail
	code, _ = httpJSON(t, ts, "POST", "/api/auth/reset-password", map[string]any{
		"token": token, "newPassword": "yetanother99",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("second reset (token reuse): expected 400, got %d", code)
	}
}

// TestAuthFlow_VerifyEmail_HappyPath: a valid token → email_verified_at is set.
// The POST variant returns {ok: true}, the GET variant does a 302 to /profile?email=verified.
func TestAuthFlow_VerifyEmail_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{AuthDevReturnVerifyToken: true})

	// Register, get the verify token
	email := uniqueEmail("verify")
	code, body := httpJSON(t, ts, "POST", "/api/auth/register", map[string]any{
		"email": email, "password": "testpass123",
	})
	if code != http.StatusOK {
		t.Fatalf("register: %d body=%v", code, body)
	}
	verifyToken, _ := body["verifyToken"].(string)
	if verifyToken == "" {
		t.Fatalf("no verifyToken (AUTH_DEV_RETURN_VERIFY_TOKEN should give it): %v", body)
	}

	// POST verify
	code, _ = httpJSON(t, ts, "POST", "/api/auth/verify-email?token="+verifyToken, nil)
	if code != http.StatusOK {
		t.Fatalf("verify: %d", code)
	}

	// DB: email_verified_at is set
	var verified bool
	_ = env.Pool.QueryRow(context.Background(),
		`select email_verified_at is not null from users where lower(email) = $1`,
		email).Scan(&verified)
	if !verified {
		t.Fatalf("email_verified_at not set after verify")
	}

	// Reusing the same token → 400
	code, _ = httpJSON(t, ts, "POST", "/api/auth/verify-email?token="+verifyToken, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("token reuse: expected 400, got %d", code)
	}
}

// TestAuthFlow_VerifyEmail_NoToken: no token → 400.
func TestAuthFlow_VerifyEmail_NoToken(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code, _ := httpJSON(t, ts, "POST", "/api/auth/verify-email", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 without token, got %d", code)
	}
}

// TestAuthFlow_VerifyEmail_BadToken: non-existent token → 400.
func TestAuthFlow_VerifyEmail_BadToken(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	code, _ := httpJSON(t, ts, "POST", "/api/auth/verify-email?token=nonexistent", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad token, got %d", code)
	}
}
