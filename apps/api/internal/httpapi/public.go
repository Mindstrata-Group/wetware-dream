package httpapi

import (
	"net/http"
	"time"

	publicapi "mindstrata-stage1/api/internal/httpapi/public"
)

type DemoModeOption = publicapi.DemoModeOption

const publicDemoModesCacheTTL = 5 * time.Minute

// clearPublicDemoModesCache resets this Handler's cache of public demo modes.
func (h Handler) clearPublicDemoModesCache() {
	if h.c == nil {
		return
	}
	h.c.publicDemoModes.Lock()
	h.c.publicDemoModes.modes = nil
	h.c.publicDemoModes.expiresAt = time.Time{}
	h.c.publicDemoModes.Unlock()
}

func cloneDemoModes(src []DemoModeOption) []DemoModeOption {
	return publicapi.CloneDemoModes(src)
}

func (h Handler) PublicDemoModes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}

	// W-NEW3-2: double-checked locking protects against a thundering herd.
	// Cold cache + 1000 concurrent requests without it = 1000 DB queries.
	// With DCL only one goroutine goes to the DB, the others wait for the write lock and
	// get the already filled cache.
	now := time.Now()
	c := &h.c.publicDemoModes

	c.RLock()
	if c.modes != nil && now.Before(c.expiresAt) {
		recordCacheLookup("public_demo_modes", true)
		out := cloneDemoModes(c.modes)
		c.RUnlock()

		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "modes": out})
		return
	}
	c.RUnlock()

	// Slow path: take the write lock and double-check after waiting.
	c.Lock()
	if c.modes != nil && time.Now().Before(c.expiresAt) {
		recordCacheLookup("public_demo_modes", true)
		out := cloneDemoModes(c.modes)
		c.Unlock()
		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "modes": out})
		return
	}

	recordCacheLookup("public_demo_modes", false)

	rows, err := h.DB.Query(r.Context(), `
		select id, name, demo_chat
		from public.get_public_demo_modes_cached()
		order by id asc`)
	if err != nil {
		c.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	out := make([]DemoModeOption, 0, 120)
	for rows.Next() {
		var item DemoModeOption
		if err := rows.Scan(&item.ID, &item.Name, &item.DemoChat); err != nil {
			rows.Close()
			c.Unlock()
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		c.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	c.modes = cloneDemoModes(out)
	c.expiresAt = time.Now().Add(publicDemoModesCacheTTL)
	c.Unlock()

	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "modes": out})
}
