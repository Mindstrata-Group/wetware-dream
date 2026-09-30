package httpapi

import "net/http"

func (h Handler) ExpertStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "expert", "admin")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"user":         user,
		"status":       "stub",
		"message":      "Expert workspace is reserved. Mode publishing will be enabled after moderation workflow is implemented.",
		"capabilities": []string{"view_expert_status", "prepare_modes_soon", "publish_after_moderation_soon"},
	})
}
