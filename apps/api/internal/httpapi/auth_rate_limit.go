package httpapi

import (
	"net/http"
	"time"
)

type authRateWindow struct {
	Count     int
	ResetTime time.Time
}

// allowAuthAttempt is a Handler method that checks the per-IP rate limit for a scope (login/register/guest/etc).
// K-NEW-7: bound to the IP only (no UA), with a periodic sweep of stale windows.
func (h Handler) allowAuthAttempt(r *http.Request, scope string, maxAttempts int, window time.Duration) bool {
	rl := &h.c.authRL
	// The client IP honours TRUSTED_PROXY_IPS: a forged X-Forwarded-For from an
	// untrusted address must not open a fresh budget.
	key := scope + ":" + h.trustedRequestIPHash(r)
	now := time.Now()
	rl.Lock()
	defer rl.Unlock()
	// Sweep stale windows at most once a minute so it is not CPU-heavy.
	if now.Sub(rl.lastSweep) > time.Minute {
		for k, w := range rl.items {
			if now.After(w.ResetTime) {
				delete(rl.items, k)
			}
		}
		rl.lastSweep = now
	}
	item := rl.items[key]
	if item.ResetTime.IsZero() || now.After(item.ResetTime) {
		rl.items[key] = authRateWindow{Count: 1, ResetTime: now.Add(window)}
		return true
	}
	if item.Count >= maxAttempts {
		return false
	}
	item.Count++
	rl.items[key] = item
	return true
}

// allowChatBurst is a Handler method that checks the chat burst limit by userID.
// K-NEW-8: protection against amplification (every message = an OpenAI API call).
func (h Handler) allowChatBurst(userID int64, maxBurst int, window time.Duration) bool {
	if userID <= 0 {
		return true
	}
	rl := &h.c.chatRL
	now := time.Now()
	rl.Lock()
	defer rl.Unlock()
	if now.Sub(rl.lastSweep) > time.Minute {
		for k, w := range rl.items {
			if now.After(w.ResetTime) {
				delete(rl.items, k)
			}
		}
		rl.lastSweep = now
	}
	item := rl.items[userID]
	if item.ResetTime.IsZero() || now.After(item.ResetTime) {
		rl.items[userID] = authRateWindow{Count: 1, ResetTime: now.Add(window)}
		return true
	}
	if item.Count >= maxBurst {
		return false
	}
	item.Count++
	rl.items[userID] = item
	return true
}
