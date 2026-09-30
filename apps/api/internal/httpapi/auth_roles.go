package httpapi

import (
	"errors"
	"net/http"
)

type AuthenticatedUser struct {
	ID     int64  `json:"id"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

var errAuthRequired = errors.New("auth required")

func validRole(role string) bool {
	switch role {
	case "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner":
		return true
	default:
		return false
	}
}

func validStatus(status string) bool {
	switch status {
	case "active", "blocked":
		return true
	default:
		return false
	}
}

func roleAllowed(role string, allowed ...string) bool {
	if role == "owner" {
		return true
	}
	for _, item := range allowed {
		if role == item {
			return true
		}
	}
	return false
}

func adminMutationAllowed(role string) bool {
	return roleAllowed(role, "owner", "admin")
}

func adminSectionAllowed(role, section string, mutation bool) bool {
	if adminMutationAllowed(role) {
		return true
	}
	if mutation {
		return false
	}
	switch section {
	case "stats", "system":
		return roleAllowed(role, "billing_admin", "support")
	case "users", "access", "dialogs":
		return roleAllowed(role, "support")
	case "modes", "orchestration":
		return roleAllowed(role, "content_admin")
	case "exports":
		return roleAllowed(role, "support")
	case "tariffs", "billing":
		return roleAllowed(role, "billing_admin")
	default:
		return false
	}
}

func (h Handler) requireRole(w http.ResponseWriter, r *http.Request, allowed ...string) (AuthenticatedUser, bool) {
	w.Header().Set("Cache-Control", "no-store")
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "auth required"})
		return AuthenticatedUser{}, false
	}
	if !roleAllowed(user.Role, allowed...) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return AuthenticatedUser{}, false
	}
	return user, true
}

func (h Handler) requireAdminSection(w http.ResponseWriter, r *http.Request, section string, mutation bool) (AuthenticatedUser, bool) {
	user, ok := h.requireRole(w, r, "owner", "admin", "billing_admin", "content_admin", "support")
	if !ok {
		return AuthenticatedUser{}, false
	}
	if !adminSectionAllowed(user.Role, section, mutation) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return AuthenticatedUser{}, false
	}
	return user, true
}

func (h Handler) requireOwnerAdmin(w http.ResponseWriter, r *http.Request) (AuthenticatedUser, bool) {
	return h.requireRole(w, r, "owner", "admin")
}
