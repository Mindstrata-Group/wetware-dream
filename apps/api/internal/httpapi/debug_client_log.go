package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type clientLogEvent struct {
	Type         string `json:"type"`
	TS           int64  `json:"ts,omitempty"` // unix ms
	Composing    *bool  `json:"composing,omitempty"`
	DomLen       *int   `json:"domLen,omitempty"`
	StateLen     *int   `json:"stateLen,omitempty"`
	WasComposing *bool  `json:"wasComposing,omitempty"`
	Extra        string `json:"extra,omitempty"`
}

type clientLogBatch struct {
	Events []clientLogEvent `json:"events"`
	UA     string           `json:"ua"`
	Path   string           `json:"path"`
}

type clientLogEntry struct {
	At     time.Time
	UserID int64
	Role   string
	IP     string
	UA     string
	Path   string
	Events []clientLogEvent
}

var (
	clientLogMu      sync.Mutex
	clientLogEnabled bool // off by default, turned on from /api/admin/client-logs (POST)
	clientLogEntries []clientLogEntry
)

const clientLogMaxEntries = 5000
const clientLogTTL = 24 * time.Hour

func clientLogAppend(entry clientLogEntry) {
	clientLogMu.Lock()
	defer clientLogMu.Unlock()
	cutoff := time.Now().Add(-clientLogTTL)
	start := 0
	for start < len(clientLogEntries) && clientLogEntries[start].At.Before(cutoff) {
		start++
	}
	clientLogEntries = clientLogEntries[start:]
	clientLogEntries = append(clientLogEntries, entry)
	if len(clientLogEntries) > clientLogMaxEntries {
		clientLogEntries = clientLogEntries[len(clientLogEntries)-clientLogMaxEntries:]
	}
}

func clientLogGetAll() []clientLogEntry {
	clientLogMu.Lock()
	defer clientLogMu.Unlock()
	cutoff := time.Now().Add(-clientLogTTL)
	start := 0
	for start < len(clientLogEntries) && clientLogEntries[start].At.Before(cutoff) {
		start++
	}
	clientLogEntries = clientLogEntries[start:]
	out := make([]clientLogEntry, len(clientLogEntries))
	copy(out, clientLogEntries)
	return out
}

// POST /api/debug/client-log accepts textarea events from the client.
// Silently discards events when clientLogEnabled == false.
func (h Handler) DebugClientLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// If logging is off, just return 200 (a silent no-op).
	clientLogMu.Lock()
	enabled := clientLogEnabled
	clientLogMu.Unlock()
	if !enabled {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	userID, _, _, err := h.ensureGuestUser(r.Context(), w, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	role, _ := h.userRole(r.Context(), userID)

	body, readErr := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if readErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read error"})
		return
	}
	var batch clientLogBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if len(batch.Events) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	// Limit: at most 200 events per request
	if len(batch.Events) > 200 {
		batch.Events = batch.Events[:200]
	}
	ip := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip = xff
	}
	clientLogAppend(clientLogEntry{
		At:     time.Now(),
		UserID: userID,
		Role:   role,
		IP:     ip,
		UA:     batch.UA,
		Path:   batch.Path,
		Events: batch.Events,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /api/admin/client-logs returns the accumulated events and the enabled flag.
// POST /api/admin/client-logs toggles logging {"enabled": bool}.
// admin/owner only.
func (h Handler) AdminClientLogs(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, 256))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		clientLogMu.Lock()
		clientLogEnabled = body.Enabled
		if !clientLogEnabled {
			clientLogEntries = nil // clear the buffer when turning it off
		}
		clientLogMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": body.Enabled})

	case http.MethodGet:
		clientLogMu.Lock()
		enabled := clientLogEnabled
		clientLogMu.Unlock()

		entries := clientLogGetAll()
		type outEvent = clientLogEvent
		type outEntry struct {
			At     string     `json:"at"`
			UserID int64      `json:"userId"`
			Role   string     `json:"role"`
			UA     string     `json:"ua"`
			Path   string     `json:"path"`
			Events []outEvent `json:"events"`
		}
		out := make([]outEntry, len(entries))
		for i, e := range entries {
			out[i] = outEntry{
				At:     e.At.UTC().Format(time.RFC3339),
				UserID: e.UserID,
				Role:   e.Role,
				UA:     e.UA,
				Path:   e.Path,
				Events: e.Events,
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled, "entries": out})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
