package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"user@example.com", "user@example.com"},
		{"  User@Example.COM  ", "user@example.com"},
		{"USER@EXAMPLE.COM", "user@example.com"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := normalizeEmail(tc.in); got != tc.want {
			t.Errorf("normalizeEmail(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidRole(t *testing.T) {
	t.Parallel()

	valid := []string{"user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner"}
	for _, r := range valid {
		if !validRole(r) {
			t.Errorf("validRole(%q) should be true", r)
		}
	}
	invalid := []string{"", "superuser", "moderator", "guest", "USER"}
	for _, r := range invalid {
		if validRole(r) {
			t.Errorf("validRole(%q) should be false", r)
		}
	}
}

func TestValidStatus(t *testing.T) {
	t.Parallel()

	if !validStatus("active") {
		t.Error("active should be valid")
	}
	if !validStatus("blocked") {
		t.Error("blocked should be valid")
	}
	for _, s := range []string{"", "pending", "deleted", "ACTIVE"} {
		if validStatus(s) {
			t.Errorf("validStatus(%q) should be false", s)
		}
	}
}

func TestRoleAllowed(t *testing.T) {
	t.Parallel()

	// owner always wins regardless of allowed list
	if !roleAllowed("owner") {
		t.Error("owner must always be allowed")
	}
	if !roleAllowed("owner", "user") {
		t.Error("owner must always be allowed even if not in list")
	}

	if !roleAllowed("admin", "admin", "support") {
		t.Error("admin should match allowed list")
	}
	if roleAllowed("user", "admin", "support") {
		t.Error("user not in allowed list should return false")
	}
	if roleAllowed("", "admin") {
		t.Error("empty role should return false")
	}
}

func TestAdminMutationAllowed(t *testing.T) {
	t.Parallel()

	allowed := []string{"owner", "admin"}
	for _, r := range allowed {
		if !adminMutationAllowed(r) {
			t.Errorf("adminMutationAllowed(%q) should be true", r)
		}
	}
	denied := []string{"billing_admin", "content_admin", "support", "user", "tester"}
	for _, r := range denied {
		if adminMutationAllowed(r) {
			t.Errorf("adminMutationAllowed(%q) should be false", r)
		}
	}
}

func TestAdminSectionAllowed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		role, section string
		mutation      bool
		want          bool
	}{
		// owner can do anything
		{"owner", "stats", false, true},
		{"owner", "tariffs", true, true},
		// admin can do anything
		{"admin", "modes", true, true},
		{"admin", "users", false, true},
		// reads by section
		{"support", "stats", false, true},
		{"support", "system", false, true},
		{"support", "users", false, true},
		{"support", "access", false, true},
		{"support", "dialogs", false, true},
		{"billing_admin", "stats", false, true},
		{"billing_admin", "tariffs", false, true},
		{"billing_admin", "billing", false, true},
		{"content_admin", "modes", false, true},
		{"content_admin", "orchestration", false, true},
		{"content_admin", "exports", false, false},
		{"support", "exports", false, true},
		// mutations denied for non owner/admin
		{"support", "users", true, false},
		{"billing_admin", "tariffs", true, false},
		{"content_admin", "modes", true, false},
		// wrong section
		{"support", "tariffs", false, false},
		{"billing_admin", "modes", false, false},
		{"user", "stats", false, false},
		// unknown section
		{"admin", "unknown", false, true}, // admin can do anything
		{"support", "unknown", false, false},
	}
	for _, tc := range cases {
		got := adminSectionAllowed(tc.role, tc.section, tc.mutation)
		if got != tc.want {
			t.Errorf("adminSectionAllowed(%q, %q, mutation=%v) = %v, want %v",
				tc.role, tc.section, tc.mutation, got, tc.want)
		}
	}
}

func TestTokenHash(t *testing.T) {
	t.Parallel()

	// must be deterministic
	h1 := tokenHash("hello")
	h2 := tokenHash("hello")
	if h1 != h2 {
		t.Error("tokenHash should be deterministic")
	}
	// different inputs → different hashes
	if tokenHash("a") == tokenHash("b") {
		t.Error("tokenHash should differ for different inputs")
	}
	// must be 64 hex chars (SHA-256)
	if len(tokenHash("test")) != 64 {
		t.Errorf("tokenHash length = %d, want 64", len(tokenHash("test")))
	}
}

func TestClientIP(t *testing.T) {
	t.Parallel()

	make := func(xff, xri, remote string) *httptest.ResponseRecorder {
		_ = xff
		return nil
	}
	_ = make

	cases := []struct {
		xff, xri, remote string
		want             string
	}{
		// X-Forwarded-For: first IP wins
		{"203.0.113.5, 10.0.0.1", "", "127.0.0.1:9090", "203.0.113.5"},
		// X-Real-IP fallback
		{"", "203.0.113.7", "127.0.0.1:9090", "203.0.113.7"},
		// RemoteAddr fallback (host:port)
		{"", "", "203.0.113.9:1234", "203.0.113.9"},
		// RemoteAddr without port
		{"", "", "203.0.113.9", "203.0.113.9"},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		if tc.xff != "" {
			r.Header.Set("X-Forwarded-For", tc.xff)
		}
		if tc.xri != "" {
			r.Header.Set("X-Real-IP", tc.xri)
		}
		if got := clientIP(r); got != tc.want {
			t.Errorf("clientIP(xff=%q, xri=%q, remote=%q) = %q, want %q",
				tc.xff, tc.xri, tc.remote, got, tc.want)
		}
	}
}
