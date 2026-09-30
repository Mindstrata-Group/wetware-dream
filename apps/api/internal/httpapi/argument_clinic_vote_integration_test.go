//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/modules/argumentclinic"
	"mindstrata-stage1/api/internal/testsupport"
)

func TestArgumentClinicVote_StoresEmailForFollowUpAndRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	body := []byte(`{"email":"Scholar@Example.EDU","vote":"virginia_ega"}`)
	resp, err := ts.Client.Post(ts.URL("/api/argument-clinic/vote"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post vote: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first vote status=%d, want 200", resp.StatusCode)
	}
	var firstBody struct {
		OK     bool             `json:"ok"`
		Vote   string           `json:"vote"`
		Counts map[string]int64 `json:"counts"`
		Total  int64            `json:"total"`
	}
	decodeResponse(t, resp, &firstBody)
	if !firstBody.OK || firstBody.Vote != "virginia_ega" || firstBody.Total != 1 || firstBody.Counts["virginia_ega"] != 1 {
		t.Fatalf("first response=%+v, want vote count for virginia_ega", firstBody)
	}

	var emailHash, email, vote string
	if err := env.Pool.QueryRow(context.Background(), `
		select email_hash, email, vote from argument_clinic_votes`).Scan(&emailHash, &email, &vote); err != nil {
		t.Fatalf("read vote: %v", err)
	}
	if emailHash != argumentclinic.EmailHash("scholar@example.edu") {
		t.Fatalf("email_hash=%q, want deterministic normalized hash", emailHash)
	}
	if email != "scholar@example.edu" {
		t.Fatalf("email=%q, want normalized address for follow-up", email)
	}
	if vote != "virginia_ega" {
		t.Fatalf("vote=%q, want virginia_ega", vote)
	}

	resp, err = ts.Client.Post(ts.URL("/api/argument-clinic/vote"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post duplicate vote: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status=%d, want 409", resp.StatusCode)
	}
	var duplicateBody struct {
		OK     bool             `json:"ok"`
		Code   string           `json:"code"`
		Counts map[string]int64 `json:"counts"`
		Total  int64            `json:"total"`
	}
	decodeResponse(t, resp, &duplicateBody)
	if duplicateBody.OK || duplicateBody.Code != "already_voted" || duplicateBody.Total != 1 || duplicateBody.Counts["virginia_ega"] != 1 {
		t.Fatalf("duplicate response=%+v, want stable aggregate counts", duplicateBody)
	}
}

func TestArgumentClinicVote_CurrentBallotIgnoresLegacyVotes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	if _, err := env.Pool.Exec(context.Background(), `
		insert into argument_clinic_votes (email_hash, vote)
		values ('legacy-email-1', 'bridge'), ('legacy-email-2', 'virginia_ega')`); err != nil {
		t.Fatalf("insert legacy votes: %v", err)
	}

	resp, err := ts.Client.Get(ts.URL("/api/argument-clinic/vote"))
	if err != nil {
		t.Fatalf("get vote results: %v", err)
	}
	var before struct {
		OK     bool             `json:"ok"`
		Counts map[string]int64 `json:"counts"`
		Total  int64            `json:"total"`
	}
	decodeResponse(t, resp, &before)
	if !before.OK || before.Total != 0 || before.Counts["bridge"] != 0 || before.Counts["virginia_ega"] != 0 {
		t.Fatalf("legacy-only results=%+v, want empty current ballot", before)
	}

	resp, err = ts.Client.Post(ts.URL("/api/argument-clinic/vote"), "application/json", bytes.NewReader([]byte(
		`{"email":"legacy-voter@example.edu","vote":"bridge"}`,
	)))
	if err != nil {
		t.Fatalf("post current vote: %v", err)
	}
	var after struct {
		OK     bool             `json:"ok"`
		Counts map[string]int64 `json:"counts"`
		Total  int64            `json:"total"`
	}
	decodeResponse(t, resp, &after)
	if !after.OK || after.Total != 1 || after.Counts["bridge"] != 1 || after.Counts["virginia_ega"] != 0 {
		t.Fatalf("current results=%+v, want one bridge vote", after)
	}
}

func TestArgumentClinicVote_ReturnsAggregateResults(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	votes := []string{
		`{"email":"one@example.edu","vote":"virginia_ega"}`,
		`{"email":"two@example.edu","vote":"bridge"}`,
		`{"email":"three@example.edu","vote":"bridge"}`,
	}
	for _, body := range votes {
		resp, err := ts.Client.Post(ts.URL("/api/argument-clinic/vote"), "application/json", bytes.NewReader([]byte(body)))
		if err != nil {
			t.Fatalf("post vote: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			t.Fatalf("post vote status=%d body=%s, want 200", resp.StatusCode, string(raw))
		}
		_ = resp.Body.Close()
	}

	resp, err := ts.Client.Get(ts.URL("/api/argument-clinic/vote"))
	if err != nil {
		t.Fatalf("get vote results: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get results status=%d, want 200", resp.StatusCode)
	}
	var body struct {
		OK     bool             `json:"ok"`
		Counts map[string]int64 `json:"counts"`
		Total  int64            `json:"total"`
	}
	decodeResponse(t, resp, &body)
	if !body.OK || body.Total != 3 || body.Counts["virginia_ega"] != 1 || body.Counts["bridge"] != 2 || body.Counts["amsterdam_network_psychometrics"] != 0 {
		t.Fatalf("results=%+v, want aggregate counts for all choices", body)
	}
}

func TestArgumentClinicVote_RejectsInvalidPayloads(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	cases := []struct {
		name string
		body map[string]string
	}{
		{name: "bad email", body: map[string]string{"email": "not-email", "vote": "bridge"}},
		{name: "unknown vote", body: map[string]string{"email": "a@example.edu", "vote": "other"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			resp, err := ts.Client.Post(ts.URL("/api/argument-clinic/vote"), "application/json", bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("post vote: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400", resp.StatusCode)
			}
		})
	}
}

func decodeResponse(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
