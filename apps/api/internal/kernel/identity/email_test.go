package identity

import "testing"

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"  User@Example.COM ": "user@example.com",
		"a@b.c":               "a@b.c",
		"":                    "",
	}
	for in, want := range cases {
		if got := NormalizeEmail(in); got != want {
			t.Fatalf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
