package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSafePublicReturnURL_AllowsOnlyMindstrataOrigins(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "https://stage.mindstrata.ru/api/payments/yookassa/create", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "stage.mindstrata.ru")

	if got := safePublicReturnURL(req, "https://stage.mindstrata.ru/profile", "/profile"); got != "https://stage.mindstrata.ru/profile" {
		t.Fatalf("allowed return URL=%q, want stage profile", got)
	}
	if got := safePublicReturnURL(req, "/profile?tab=billing", "/profile"); got != "https://stage.mindstrata.ru/profile?tab=billing" {
		t.Fatalf("relative return URL=%q, want stage billing profile", got)
	}
	if got := safePublicReturnURL(req, "https://evil.test/profile", "/profile"); got != "https://stage.mindstrata.ru/profile" {
		t.Fatalf("external return URL fallback=%q, want stage profile", got)
	}
}

func TestPublicReturnURL_DoesNotTrustSpoofedForwardedHost(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "https://mindstrata.ru/api/payments/yookassa/create", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "evil.test")

	if got := publicReturnURL(req, "/profile"); got != "https://mindstrata.ru/profile" {
		t.Fatalf("spoofed forwarded host fallback=%q, want request host profile", got)
	}
}
