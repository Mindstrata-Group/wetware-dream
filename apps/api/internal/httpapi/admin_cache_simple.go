package httpapi

import (
	"net/http"
	"sync"
	"time"
)

// simpleGlobalCache is a shared one-slot in-memory cache for admin endpoints
// without query parameters (such as /api/admin/ai-settings, /api/admin/status).
// Parameterised endpoints (by q/limit/offset) use their own map caches.
type simpleGlobalCache struct {
	mu        sync.Mutex
	expiresAt time.Time
	body      []byte
}

func (c *simpleGlobalCache) get() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.body != nil && time.Now().Before(c.expiresAt) {
		return c.body
	}
	return nil
}

func (c *simpleGlobalCache) set(body []byte, ttl time.Duration) {
	c.mu.Lock()
	c.body = body
	c.expiresAt = time.Now().Add(ttl)
	c.mu.Unlock()
}

func (c *simpleGlobalCache) clear() {
	c.mu.Lock()
	c.body = nil
	c.expiresAt = time.Time{}
	c.mu.Unlock()
}

// writeCached: if the body is cached, send it as JSON and return true.
// Otherwise return false and the handler goes to the DB itself.
func writeCached(w http.ResponseWriter, c *simpleGlobalCache) bool {
	return writeNamedCached(w, "simple_global", c)
}

func writeNamedCached(w http.ResponseWriter, name string, c *simpleGlobalCache) bool {
	body := c.get()
	hit := body != nil
	recordCacheLookup(name, hit)
	if !hit {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
	return true
}

const adminConfigCacheTTL = 60 * time.Second
