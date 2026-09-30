package argumentclinic

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Database-backed behaviour is covered end to end by
// internal/httpapi/argument_clinic_vote_integration_test.go through the real
// router; these tests cover what does not need a database.

func TestVote_WithoutDatabase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, method string
		want         int
	}{
		{"get without db", http.MethodGet, http.StatusServiceUnavailable},
		{"post without db", http.MethodPost, http.StatusServiceUnavailable},
		{"put not allowed", http.MethodPut, http.StatusMethodNotAllowed},
		{"delete not allowed", http.MethodDelete, http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/api/argument-clinic/vote", strings.NewReader(`{"email":"a@b.c","vote":"bridge"}`))
			Module{}.Vote(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestVoteAllowed(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"virginia_ega", "amsterdam_network_psychometrics", "bridge"} {
		if !VoteAllowed(v) {
			t.Fatalf("VoteAllowed(%q) = false", v)
		}
	}
	for _, v := range []string{"", "Bridge", "other"} {
		if VoteAllowed(v) {
			t.Fatalf("VoteAllowed(%q) = true", v)
		}
	}
}

func TestEmailHash_IsBallotScopedAndNormalized(t *testing.T) {
	t.Parallel()
	a := EmailHash("  Scholar@Example.EDU ")
	b := EmailHash("scholar@example.edu")
	if a != b {
		t.Fatalf("hash depends on case/spaces: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, BallotVersion+":") || len(a) != len(BallotVersion)+1+64 {
		t.Fatalf("unexpected hash format %q", a)
	}
	if EmailHash("other@example.edu") == a {
		t.Fatal("different emails share a hash")
	}
}
