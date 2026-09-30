// Package argumentclinic is the public vote widget ("Argument Clinic"):
// one email, one vote, aggregated results.
//
// It is the pilot module of the modular monolith (ADR-0001): it owns the
// argument_clinic_votes table, depends only on the kernel, and exposes its
// HTTP handler through Module. The route itself stays in the central route
// table (internal/httpapi/router.go), which the route inventory and OpenAPI
// tests read.
package argumentclinic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/mail"

	"github.com/jackc/pgx/v5/pgxpool"

	"mindstrata-stage1/api/internal/kernel/httpjson"
	"mindstrata-stage1/api/internal/kernel/identity"
)

// BallotVersion prefixes stored email hashes; votes of older ballots are
// kept but no longer counted.
const BallotVersion = "argument-clinic-v2"

// Module serves the vote endpoint. A nil DB answers 503, as before the move.
type Module struct {
	DB *pgxpool.Pool
}

type voteRequest struct {
	Email string `json:"email"`
	Vote  string `json:"vote"`
}

// Vote handles GET (results) and POST (cast a vote) on /api/argument-clinic/vote.
func (m Module) Vote(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		m.results(w, r)
		return
	}
	if r.Method != http.MethodPost {
		httpjson.Write(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if m.DB == nil {
		httpjson.Write(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}

	var req voteRequest
	if err := httpjson.DecodeStrict(w, r, 8<<10, &req); err != nil {
		httpjson.Write(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	email := identity.NormalizeEmail(req.Email)
	if email == "" || !VoteAllowed(req.Vote) {
		httpjson.Write(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "email and vote are required"})
		return
	}
	if parsed, err := mail.ParseAddress(email); err != nil || identity.NormalizeEmail(parsed.Address) != email {
		httpjson.Write(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "valid email is required"})
		return
	}

	cmd, err := m.DB.Exec(r.Context(), `
		insert into argument_clinic_votes (email_hash, email, vote)
		values ($1, $2, $3)
		on conflict (email_hash) do nothing`,
		EmailHash(email), email, req.Vote,
	)
	if err != nil {
		httpjson.Write(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	counts, total, countErr := m.counts(r.Context())
	if cmd.RowsAffected() == 0 {
		body := map[string]any{"ok": false, "error": "email already voted", "code": "already_voted"}
		if countErr == nil {
			body["counts"] = counts
			body["total"] = total
		}
		httpjson.Write(w, http.StatusConflict, body)
		return
	}
	if countErr != nil {
		httpjson.Write(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": countErr.Error()})
		return
	}

	httpjson.Write(w, http.StatusOK, map[string]any{"ok": true, "vote": req.Vote, "counts": counts, "total": total})
}

func (m Module) results(w http.ResponseWriter, r *http.Request) {
	if m.DB == nil {
		httpjson.Write(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	counts, total, err := m.counts(r.Context())
	if err != nil {
		httpjson.Write(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"ok": true, "counts": counts, "total": total})
}

func (m Module) counts(ctx context.Context) (map[string]int64, int64, error) {
	counts := map[string]int64{
		"virginia_ega":                    0,
		"amsterdam_network_psychometrics": 0,
		"bridge":                          0,
	}
	rows, err := m.DB.Query(ctx, `
		select vote, count(*)::bigint
		from argument_clinic_votes
		where email_hash like $1
		group by vote`,
		BallotVersion+":%",
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var total int64
	for rows.Next() {
		var vote string
		var count int64
		if err := rows.Scan(&vote, &count); err != nil {
			return nil, 0, err
		}
		if VoteAllowed(vote) {
			counts[vote] = count
			total += count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return counts, total, nil
}

// VoteAllowed reports whether v is one of the ballot options.
func VoteAllowed(v string) bool {
	switch v {
	case "virginia_ega", "amsterdam_network_psychometrics", "bridge":
		return true
	default:
		return false
	}
}

// EmailHash is the stored, ballot-scoped identity of a voter.
func EmailHash(email string) string {
	sum := sha256.Sum256([]byte(BallotVersion + ":" + identity.NormalizeEmail(email)))
	return BallotVersion + ":" + hex.EncodeToString(sum[:])
}
