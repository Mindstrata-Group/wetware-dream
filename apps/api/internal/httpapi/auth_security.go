package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"mindstrata-stage1/api/internal/kernel/identity"
)

// Note: the original auth_security.go was ~16 KB; it was split into:
//   - auth_password.go  - hash/verify, dummy timing, PBKDF2 constants
//   - auth_session.go   - session cookies, createAuthSession, currentUser*
//   - auth_roles.go     - validRole/requireRole/adminSection*, AuthenticatedUser
//   - auth_rate_limit.go- authRateLimiter, allowAuthAttempt, chatRateLimiter
//   - auth_client_ip.go - clientIP, trusted proxies, isHTTPSRequest
//   - auth_security.go  - shared utilities (this file): randomToken, tokenHash,
//                          normalizeEmail, parseIDFromPath, writeAdminAudit,
//                          scanOptionalNoRows.
// SOLID split 2026-05-31: each file has a single responsibility.

func normalizeEmail(v string) string {
	return identity.NormalizeEmail(v)
}

func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func parseIDFromPath(path, prefix string) (int64, string, error) {
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rest == "" {
		return 0, "", errors.New("missing id")
	}
	parts := strings.Split(rest, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", errors.New("invalid id")
	}
	suffix := ""
	if len(parts) > 1 {
		suffix = strings.Join(parts[1:], "/")
	}
	return id, suffix, nil
}

func (h Handler) writeAdminAudit(ctx context.Context, r *http.Request, actorID int64, action, targetType string, targetID *int64, meta map[string]any) {
	// actor_user_id references users(id), so a zero actor must become
	// NULL: otherwise the insert fails on the foreign key, the error is swallowed in this
	// same function, and the action leaves no trace at all. Zero means "there is no
	// human behind this action": a teacher key, an MCP agent, a background job.
	var actor any
	if actorID > 0 {
		actor = actorID
	}
	path := auditPath(r)
	class := classifyAdminAudit(action, targetType, path)
	method := ""
	userAgent := ""
	if r != nil {
		method = r.Method
		userAgent = strings.TrimSpace(r.UserAgent())
	}
	metaJSON := marshalAuditJSON(meta)
	if _, err := h.DB.Exec(ctx, `
		insert into admin_audit_log (
			actor_user_id, action, target_type, target_id, ip_hash, user_agent, meta,
			request_id, outcome, http_method, path, section, sensitive,
			before_state, after_state, changed_fields, error_code
		)
		values ($1, $2, $3, $4, $5, $6, $7::jsonb,
		        $8, 'success', $9, $10, $11, $12,
		        '{}'::jsonb, '{}'::jsonb, '{}'::text[], null)`,
		actor,
		action,
		targetType,
		targetID,
		requestIPHash(r),
		userAgent,
		marshalAuditJSON(meta),
		requestIDFromContext(ctx),
		method,
		path,
		class.Section,
		class.Sensitive,
	); err != nil {
		_, _ = h.DB.Exec(ctx, `
			insert into admin_audit_log (actor_user_id, action, target_type, target_id, ip_hash, user_agent, meta)
			values ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
			actor,
			action,
			targetType,
			targetID,
			requestIPHash(r),
			userAgent,
			metaJSON,
		)
	}
}

func scanOptionalNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
