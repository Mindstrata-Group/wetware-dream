package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	cmsapi "mindstrata-stage1/api/internal/httpapi/cms"
)

// site_content is a "key -> jsonb value" table. The public GET is cached
// in memory for 5 min (like demo-modes). Any admin change clears the cache.

const siteContentCacheTTL = 5 * time.Minute

func (h Handler) clearSiteContentCache() {
	if h.c == nil {
		return
	}
	h.c.siteContent.Lock()
	h.c.siteContent.data = nil
	h.c.siteContent.expiresAt = time.Time{}
	h.c.siteContent.Unlock()
}

// PublicSiteContent: GET /api/public/site-content, the public list
// (all site content in one response so the landing page makes 1 fetch).
//
// Thundering herd protection: right after an admin save (cache=nil) hundreds of
// simultaneous GETs may arrive. To avoid N DB queries we read under
// a write lock with a double-checked condition: only one goroutine goes to the DB,
// the others get the cache right after.
func (h Handler) PublicSiteContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}

	// Fast path: the cache is still alive.
	now := time.Now()
	c := &h.c.siteContent
	c.RLock()
	if c.data != nil && now.Before(c.expiresAt) {
		out := cloneSiteContent(c.data)
		c.RUnlock()
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "content": out})
		return
	}
	c.RUnlock()

	// Slow path: the cache expired or was reset by the admin. Take the write lock,
	// re-check under it (a parallel request may have already filled the cache),
	// otherwise go to the DB ourselves. This is double-checked locking.
	c.Lock()
	if c.data != nil && time.Now().Before(c.expiresAt) {
		out := cloneSiteContent(c.data)
		c.Unlock()
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "content": out})
		return
	}

	rows, err := h.DB.Query(r.Context(), `select key, value from site_content`)
	if err != nil {
		c.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	out := make(map[string]json.RawMessage, 32)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			c.Unlock()
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out[key] = json.RawMessage(value)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		c.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	c.data = cloneSiteContent(out)
	c.expiresAt = time.Now().Add(siteContentCacheTTL)
	c.Unlock()

	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "content": out})
}

func cloneSiteContent(src map[string]json.RawMessage) map[string]json.RawMessage {
	return cmsapi.CloneSiteContent(src)
}

// AdminSiteContentDispatch: GET list / POST upsert. One route /api/admin/site-content
// dispatches by method.
func (h Handler) AdminSiteContentDispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.AdminSiteContentList(w, r)
	case http.MethodPost:
		h.AdminSiteContent(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

// AdminSiteContentFlush: POST /api/admin/site-content/flush forcibly
// resets the in-memory cache. Useful when the admin needs changes applied
// right now without waiting for the 5 min TTL.
func (h Handler) AdminSiteContentFlush(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "modes", true)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	h.clearSiteContentCache()
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.site_content.flush", "site_content", nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "flushed": true})
}

// AdminSiteContent: POST /api/admin/site-content upserts one key.
// Body: {"key": "landing.hero.title", "value": <any-json>}
// The cache is cleared after a successful write.
func (h Handler) AdminSiteContent(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "modes", true)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	var req struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if err := decodeJSONStrict(w, r, 256<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	key := strings.TrimSpace(req.Key)
	if key == "" || len(key) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "key required (≤200 chars)"})
		return
	}
	if len(req.Value) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "value required"})
		return
	}
	// Validate that value is valid JSON.
	var tmp any
	if err := json.Unmarshal(req.Value, &tmp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "value must be valid JSON"})
		return
	}

	_, err := h.DB.Exec(r.Context(),
		`insert into site_content (key, value, updated_by, updated_at)
		 values ($1, $2::jsonb, $3, now())
		 on conflict (key) do update set
		    value = excluded.value,
		    updated_by = excluded.updated_by,
		    updated_at = now()`,
		key, string(req.Value), actor.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	h.clearSiteContentCache()
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.site_content.update", "site_content", nil, map[string]any{"key": key})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AdminSiteContentList: GET /api/admin/site-content returns all records for editing.
// Unlike the public one it returns meta (updated_at).
func (h Handler) AdminSiteContentList(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdminSection(w, r, "modes", false); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		select key, value, updated_at
		from site_content
		order by key asc`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type item struct {
		Key       string          `json:"key"`
		Value     json.RawMessage `json:"value"`
		UpdatedAt time.Time       `json:"updatedAt"`
	}
	out := []item{}
	for rows.Next() {
		var it item
		var val []byte
		if err := rows.Scan(&it.Key, &val, &it.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		it.Value = json.RawMessage(val)
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": out})
}
