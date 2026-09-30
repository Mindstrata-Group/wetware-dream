package httpapi

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzNormalizeEmail: normalizeEmail must:
//   - never panic on arbitrary input,
//   - return only valid UTF-8,
//   - be idempotent: f(f(x)) == f(x),
//   - never return mixed case (always lowercase),
//   - never return strings with leading/trailing whitespace.
func FuzzNormalizeEmail(f *testing.F) {
	seeds := []string{
		"user@example.com",
		"USER@EXAMPLE.COM",
		"  user@example.com  ",
		"",
		" ",
		"@",
		"a@",
		"@b",
		"user@\x00null.com",
		strings.Repeat("a", 10000) + "@b.com",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := normalizeEmail(in)
		if !utf8.ValidString(out) {
			t.Errorf("output not valid UTF-8: %q → %q", in, out)
		}
		if again := normalizeEmail(out); again != out {
			t.Errorf("not idempotent: %q → %q → %q", in, out, again)
		}
		if out != strings.TrimSpace(out) {
			t.Errorf("leading/trailing whitespace not stripped: %q → %q", in, out)
		}
		if out != strings.ToLower(out) {
			t.Errorf("non-lowercase output: %q → %q", in, out)
		}
	})
}

// FuzzSanitizeUserText: sanitizeUserText must:
//   - never panic,
//   - return valid UTF-8,
//   - never contain \r\n (normalized to \n),
//   - never contain 3+ consecutive newlines.
func FuzzSanitizeUserText(f *testing.F) {
	seeds := []string{
		"hello",
		"hello\nworld",
		"hello\r\nworld",
		"\n\n\n\n\nmultiple",
		"",
		" \t\n ",
		strings.Repeat("a\n", 1000),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := sanitizeUserText(in)
		if !utf8.ValidString(out) {
			t.Errorf("output not valid UTF-8: %q → %q", in, out)
		}
		if strings.Contains(out, "\r") {
			t.Errorf("CR not stripped: %q → %q", in, out)
		}
		if strings.Contains(out, "\n\n\n") {
			t.Errorf("3+ consecutive newlines not collapsed: %q → %q", in, out)
		}
	})
}

// FuzzSafeRedirectAfter: critical security boundary.
// Must never return:
//   - a string not starting with "/",
//   - protocol-relative URL (//evil.com),
//   - paths leaking into /admin, /tester, /expert,
//   - strings containing backslashes (Windows path tricks),
//   - empty (always falls back to /profile).
func FuzzSafeRedirectAfter(f *testing.F) {
	seeds := []string{
		"/profile",
		"//evil.com",
		"https://attacker.com",
		`\\evil`,
		"/admin",
		"/admin/",
		"/admin/users",
		"/tester",
		"/expert",
		"",
		"/",
		"/safe",
		"/chat?next=//bad",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := safeRedirectAfter(in)
		if out == "" {
			t.Errorf("empty output for input %q (must default to /profile)", in)
		}
		if !strings.HasPrefix(out, "/") {
			t.Errorf("output doesn't start with /: %q → %q", in, out)
		}
		if strings.HasPrefix(out, "//") {
			t.Errorf("protocol-relative URL leaked: %q → %q", in, out)
		}
		if strings.Contains(out, `\`) {
			t.Errorf("backslash not stripped: %q → %q", in, out)
		}
		if strings.HasPrefix(out, "/admin") {
			t.Errorf("redirect into /admin leaked: %q → %q", in, out)
		}
		if strings.HasPrefix(out, "/tester") {
			t.Errorf("redirect into /tester leaked: %q → %q", in, out)
		}
		if strings.HasPrefix(out, "/expert") {
			t.Errorf("redirect into /expert leaked: %q → %q", in, out)
		}
	})
}

// FuzzParseIDFromPath: parseIDFromPath must:
//   - never panic,
//   - never return a non-positive ID with err == nil,
//   - reject non-numeric first segment.
func FuzzParseIDFromPath(f *testing.F) {
	type input struct {
		path, prefix string
	}
	seeds := []input{
		{"/api/admin/users/123", "/api/admin/users/"},
		{"/api/admin/users/123/access", "/api/admin/users/"},
		{"/api/admin/users/", "/api/admin/users/"},
		{"/api/admin/users", "/api/admin/users/"},
		{"/api/admin/users/-5", "/api/admin/users/"},
		{"/api/admin/users/abc", "/api/admin/users/"},
		{"/api/admin/users/9999999999999999999999", "/api/admin/users/"},
	}
	for _, s := range seeds {
		f.Add(s.path, s.prefix)
	}
	f.Fuzz(func(t *testing.T, path, prefix string) {
		id, suffix, err := parseIDFromPath(path, prefix)
		if err != nil {
			// Valid: parser rejected input. Nothing more to check.
			_ = suffix
			return
		}
		if id <= 0 {
			t.Errorf("non-positive id with nil err: path=%q prefix=%q → id=%d", path, prefix, id)
		}
	})
}
