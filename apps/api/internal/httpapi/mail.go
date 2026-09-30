package httpapi

import (
	"os"
	"strings"
)

// publicWebBaseURL is the frontend's public URL for building email verify links,
// OAuth callback redirects and so on. Falls back: PUBLIC_WEB_BASE_URL ENV ->
// NEXT_PUBLIC_WEB_BASE_URL ENV -> localhost:3000.
func publicWebBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_WEB_BASE_URL")), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("NEXT_PUBLIC_WEB_BASE_URL")), "/")
	}
	if base == "" {
		base = "http://localhost:3000"
	}
	return base
}

// apiPublicBaseURL is the API's public URL for building the OAuth callback URL.
// Falls back: API_PUBLIC_BASE_URL -> NEXT_PUBLIC_API_BASE_URL -> localhost:18080.
func (h Handler) apiPublicBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(h.APIPublicBaseURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("NEXT_PUBLIC_API_BASE_URL")), "/")
	}
	if base == "" {
		base = "http://localhost:18080"
	}
	return base
}

// NOTE: SMTP email sending was removed on 2026-05-28; email is not used in the project.
// Email verification remains as a mechanism (the email_verification_tokens table,
// the /api/auth/verify-email?token=... endpoint), but the token is not sent automatically.
// The token can be obtained via:
//   - AUTH_DEV_RETURN_VERIFY_TOKEN=true (returns the token in the POST /api/auth/register response)
//   - A DB query by an admin: SELECT token_hash FROM email_verification_tokens WHERE user_id = ?
//     (a hash, not the token itself; createEmailVerificationToken would have to be rewritten
//     to return the token in plain text, or a new one generated)
//
// If you decide to bring email back, look at this file's git log before the commit.
