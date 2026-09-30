package httpapi

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/config"
)

// spoofedRequest comes straight from an untrusted address and claims a new
// client IP in X-Forwarded-For every time, the way a brute-forcer would.
func spoofedRequest(n int) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "http://api.local/api/auth/login", nil)
	r.RemoteAddr = "203.0.113.77:40000"
	r.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(n%250+1))
	return r
}

// Regression (2026-09-30 open-source review): every rate limiter keyed on
// requestIPHash, which trusts X-Forwarded-For from anyone and ignores
// TRUSTED_PROXY_IPS. Behind our Caddy that was masked; on any install where
// the API is reachable directly, a fresh fake header reset the login,
// password-reset and promo-code limits, i.e. unlimited guessing.
func TestAllowAuthAttempt_SpoofedXFFDoesNotResetBudget(t *testing.T) {
	t.Parallel()
	h := Handler{TrustedProxyIPs: []string{"192.168.0.0/16"}, c: newHandlerCaches()}
	const max = 5
	for i := 0; i < max; i++ {
		if !h.allowAuthAttempt(spoofedRequest(i), "login", max, time.Minute) {
			t.Fatalf("attempt %d rejected too early", i+1)
		}
	}
	if h.allowAuthAttempt(spoofedRequest(max), "login", max, time.Minute) {
		t.Fatal("a new fake X-Forwarded-For must not open a fresh login budget")
	}
}

// The same holds for the tir teacher-key limiter: once the address is locked,
// a fake header must not unlock it.
func TestKeyAttemptLimiter_SpoofedXFFStaysLocked(t *testing.T) {
	t.Parallel()
	h := Handler{TrustedProxyIPs: []string{"192.168.0.0/16"}}
	// Use an address no other test touches: the key limiter is process-wide.
	mk := func(n int) *http.Request {
		r := spoofedRequest(n)
		r.RemoteAddr = "203.0.113.201:40000"
		return r
	}
	for i := 0; i < 50; i++ {
		if locked, _ := h.keyAttemptsExhausted(mk(i)); locked {
			return // locked by the real address despite the changing header
		}
		h.guardKeyAttempt(t.Context(), mk(i), "teacher")
	}
	t.Fatal("the key limiter never locked: a changing X-Forwarded-For bypasses it")
}

// Behind a trusted proxy the forwarded client IP is still what counts, so
// two real users behind Caddy do not share one budget.
func TestAllowAuthAttempt_TrustedProxyForwardedIPsAreSeparate(t *testing.T) {
	t.Parallel()
	h := Handler{TrustedProxyIPs: []string{"192.168.0.0/16"}, c: newHandlerCaches()}
	via := func(client string) *http.Request {
		r, _ := http.NewRequest(http.MethodPost, "http://api.local/api/auth/login", nil)
		r.RemoteAddr = "192.168.10.2:5555"
		r.Header.Set("X-Forwarded-For", client)
		return r
	}
	for i := 0; i < 3; i++ {
		if !h.allowAuthAttempt(via("198.51.100.1"), "login", 3, time.Minute) {
			t.Fatal("first client rejected too early")
		}
	}
	if !h.allowAuthAttempt(via("198.51.100.2"), "login", 3, time.Minute) {
		t.Fatal("a second client behind the proxy must have its own budget")
	}
}

// Production must name its proxies: with an empty list X-Forwarded-For is
// trusted from anyone (legacy behaviour kept for local runs and tests).
func TestConfigValidate_ProductionRequiresTrustedProxies(t *testing.T) {
	t.Parallel()
	cfg := config.Config{AppEnv: "production", GuestCookieSecret: "real-secret-32-chars"}
	if len(cfg.Validate()) == 0 {
		t.Fatal("production without TRUSTED_PROXY_IPS must be rejected")
	}
	cfg.TrustedProxyIPs = []string{"172.16.0.0/12"}
	if errs := cfg.Validate(); len(errs) != 0 {
		t.Fatalf("valid production config rejected: %v", errs)
	}
}
