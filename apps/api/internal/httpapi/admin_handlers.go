package httpapi

import (
	"net/http"
	"time"
)

type adminCreateUserRequest struct {
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Role     string  `json:"role"`
	Status   string  `json:"status"`
	ModeIDs  []int64 `json:"modeIds"`
	Days     int     `json:"days"`
}

type adminPatchUserRequest struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
	Role     *string `json:"role"`
	Status   *string `json:"status"`
}

type adminGrantAccessRequest struct {
	UserID            int64   `json:"userId"`
	Email             string  `json:"email"`
	ModeID            int64   `json:"modeId"`
	ModeIDs           []int64 `json:"modeIds"`
	TariffID          int64   `json:"tariffId"`
	Days              int     `json:"days"`
	ActiveTo          string  `json:"activeTo"`
	DailyMessageLimit *int64  `json:"dailyMessageLimit"`
	ReplaceActive     bool    `json:"replaceActive"`
	ResetLimits       bool    `json:"resetLimits"`
}

type adminPatchModeRequest struct {
	Name                      *string  `json:"name"`
	Prompt                    *string  `json:"prompt"`
	WelcomeMessage            *string  `json:"welcomeMessage"`
	AIModel                   *string  `json:"aiModel"`
	AIProvider                *string  `json:"aiProvider"`
	ThinkingMode              *string  `json:"thinkingMode"`
	ModelTemperature          *float64 `json:"modelTemperature"`
	DemoChat                  *string  `json:"demoChat"`
	Hidden                    *bool    `json:"hidden"`
	AudioEnabled              *bool    `json:"audioEnabled"`
	Criteria                  *string  `json:"criteria"`
	OrchestratorCheckInterval *int     `json:"orchestratorCheckInterval"`
	ReminderCount             *int     `json:"reminderCount"`
	// AIMaxTokens: the mode's response token ceiling. 0 = reset to the global
	// default (system_settings.ai_max_tokens_default). nil = do not change.
	AIMaxTokens *int `json:"aiMaxTokens"`
	// Lead notifications: the trigger "the user replied N times" sends a message
	// to managers in Max (see lead_notifications.go). nil = do not change.
	LeadNotifyEnabled     *bool   `json:"leadNotifyEnabled"`
	LeadNotifyChatIDs     *string `json:"leadNotifyChatIds"`
	LeadNotifyTelegramIDs *string `json:"leadNotifyTelegramIds"`
	LeadNotifyThreshold   *int    `json:"leadNotifyThreshold"`
}

type adminAccessModeItem struct {
	ID                int64       `json:"id"`
	ModeID            int64       `json:"modeId"`
	ModeName          string      `json:"modeName"`
	ActiveFrom        *time.Time  `json:"activeFrom,omitempty"`
	ActiveTo          *time.Time  `json:"activeTo,omitempty"`
	Status            string      `json:"status"`
	DailyMessageLimit *int64      `json:"dailyMessageLimit,omitempty"`
	Priority          int64       `json:"priority"`
	AccessType        string      `json:"accessType"`
	SourceID          *int64      `json:"sourceId,omitempty"`
	SourceLabel       string      `json:"sourceLabel"`
	Quota             *DailyQuota `json:"quota,omitempty"`
}

func (h Handler) AdminAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "access", r.Method != http.MethodGet)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req adminGrantAccessRequest
	if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	userID, err := h.resolveAdminUserID(r.Context(), req.UserID, req.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if req.ResetLimits {
		modeIDs := append([]int64{}, req.ModeIDs...)
		if req.ModeID > 0 {
			modeIDs = append(modeIDs, req.ModeID)
		}
		updated, err := h.resetAdminAccessLimits(r.Context(), actor.ID, userID, modeIDs)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.access.reset_limits", "user", &userID, map[string]any{"modeIds": uniquePositiveIDs(modeIDs), "updated": updated})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updated, "limitsReset": true})
		return
	}
	granted, extended, err := h.grantAdminAccess(r, actor.ID, userID, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "granted": granted, "extended": extended})
}

func (h Handler) AdminSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireAdminSection(w, r, "system", false)
	if !ok {
		return
	}
	counts := map[string]int64{}
	queries := map[string]string{
		"users":       "select count(*) from users where deleted_at is null",
		"activeUsers": "select count(*) from users where deleted_at is null and status = 'active'",
		"sessions":    "select count(*) from auth_sessions where revoked_at is null and expires_at > now()",
		"modes":       "select count(*) from modes where hidden_at is null",
		"dialogs":     "select count(*) from users_dialogs where deleted_at is null",
		"messages":    "select count(*) from dialogs_messages",
	}
	for key, sql := range queries {
		var n int64
		if err := h.DB.QueryRow(r.Context(), sql).Scan(&n); err == nil {
			counts[key] = n
		}
	}
	h.writeAdminAudit(r.Context(), r, user.ID, "admin.status.read", "system", nil, nil)
	// liveAIEnabled removed on 2026-05-29: controlled by the checkbox in the chat,
	// the server-side gate is OPENAI_API_KEY only. The UI no longer shows the status.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "counts": counts})
}
