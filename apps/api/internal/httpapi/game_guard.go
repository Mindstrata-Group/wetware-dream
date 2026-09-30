package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Protection of the practicum against brute force and noise.
//
// An OWASP Top 10 review of the practicum found four holes on top of the two
// that students had already used:
//
//   A01 Broken Access Control - the teacher key travelled in the query string,
//       so it ended up in proxy logs, browser history and referrers.
//   A04 Insecure Design - no rate limit at all: the key could be
//       guessed indefinitely and results sent in batches.
//   A07 Auth Failures - failed key attempts looked no different from
//       normal traffic and were not recorded anywhere.
//   A09 Logging Failures - other people's attempts left no trace at all; when
//       we needed to find out who did what, there was nothing to look at.
//
// The limiter lives in process memory, not in the database: it must be faster than
// a query and must not create load exactly where someone wants to create it. The cost
// is that counters do not survive a restart and are not shared between replicas; for a
// practicum with one group this is enough, and key guessing still hits the ceiling.

type rateBucket struct {
	count   int
	resetAt time.Time
	lockAt  time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	limit   int
	window  time.Duration
	lockout time.Duration
}

func newRateLimiter(limit int, window, lockout time.Duration) *rateLimiter {
	return &rateLimiter{buckets: map[string]*rateBucket{}, limit: limit, window: window, lockout: lockout}
}

// allow returns false if it is time to slow down, and how long to wait.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clean up stale entries so the map does not grow forever: the key here is an IP hash,
	// and a semester accumulates quite a few of them.
	if len(l.buckets) > 4096 {
		for k, b := range l.buckets {
			if now.After(b.resetAt) && now.After(b.lockAt) {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[key]
	if !ok {
		b = &rateBucket{}
		l.buckets[key] = b
	}
	if now.Before(b.lockAt) {
		return false, b.lockAt.Sub(now)
	}
	if now.After(b.resetAt) {
		b.count = 0
		b.resetAt = now.Add(l.window)
	}
	b.count++
	if b.count > l.limit {
		b.lockAt = now.Add(l.lockout)
		return false, l.lockout
	}
	return true, 0
}

// Thresholds are tuned to real usage: a seminar group of thirty
// people fits within them with room to spare, while key guessing does not.
var (
	// Keys: five misses in a row from one address, then ten minutes of silence.
	keyAttemptLimiter = newRateLimiter(5, 10*time.Minute, 10*time.Minute)
	// Game requests: 240 per minute is four times what a player needs.
	gameCallLimiter = newRateLimiter(240, time.Minute, time.Minute)
)

// rateLimitsOff disables the ceilings. Tests need it: they deliberately knock
// with foreign keys, and blocking the address would break neighbouring checks. In a
// production build it is always false.
func (h Handler) rateLimitsOff() bool { return h.DisableRateLimits }

// guardKeyAttempt is called AFTER a failed key check.
func (h Handler) guardKeyAttempt(ctx context.Context, r *http.Request, what string) {
	if h.rateLimitsOff() {
		h.logSecurityEvent(ctx, "bad_key", "alert", r, map[string]any{"what": what})
		return
	}
	ok, wait := keyAttemptLimiter.allow("key:" + h.trustedRequestIPHash(r))
	h.logSecurityEvent(ctx, "bad_key", "alert", r, map[string]any{
		"what": what, "lockedFor": wait.String(), "locked": !ok,
	})
}

// keyAttemptsExhausted: guessing is already under way, so we do not let anyone in even with
// the correct key; otherwise the ceiling is bypassed by guessing on the last attempt.
func (h Handler) keyAttemptsExhausted(r *http.Request) (bool, time.Duration) {
	if h.rateLimitsOff() {
		return false, 0
	}
	return keyAttemptsExhaustedFor(h.trustedRequestIPHash(r))
}

// keyAttemptsExhaustedFor takes the hashed client IP (see trustedRequestIPHash).
func keyAttemptsExhaustedFor(ipHash string) (bool, time.Duration) {
	now := time.Now()
	keyAttemptLimiter.mu.Lock()
	defer keyAttemptLimiter.mu.Unlock()
	b, ok := keyAttemptLimiter.buckets["key:"+ipHash]
	if !ok || now.After(b.lockAt) {
		return false, 0
	}
	return true, b.lockAt.Sub(now)
}

// guardGameCall limits the rate of game requests from one address.
func (h Handler) guardGameCall(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if h.rateLimitsOff() {
		return true
	}
	if ok, wait := gameCallLimiter.allow("game:" + h.trustedRequestIPHash(r)); !ok {
		h.logSecurityEvent(ctx, "rate_limited", "warn", r, map[string]any{"wait": wait.String()})
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"ok": false, "error": "слишком часто, подождите немного",
		})
		return false
	}
	return true
}

// publicError hides internals from the student while keeping them in the log.
//
// The DB driver's error text used to go out, revealing table and
// column names, i.e. half the work for anyone hunting for an injection.
func (h Handler) publicError(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	h.logSecurityEvent(ctx, "server_error", "warn", r, map[string]any{"err": err.Error()})
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"ok": false, "error": "внутренняя ошибка, попробуйте ещё раз",
	})
}
