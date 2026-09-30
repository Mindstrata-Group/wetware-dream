package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h Handler) AdminBroadcasts(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Audience     string  `json:"audience"`
		Message      string  `json:"message"`
		Every        string  `json:"every"`
		PromocodeIDs []int64 `json:"promocodeIds"`
		UserIDs      []int64 `json:"userIds"`
	}
	if err := decodeJSONStrict(w, r, 128<<10, &req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "message is required"})
		return
	}
	var count int64
	switch req.Audience {
	case "active_access":
		_ = h.DB.QueryRow(r.Context(), `select count(distinct user_id) from user_mode_access where active_from <= now() and (active_to is null or active_to >= now())`).Scan(&count)
	case "promocode":
		ids := uniquePositiveIDs(req.PromocodeIDs)
		if len(ids) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "promocodeIds are required"})
			return
		}
		_ = h.DB.QueryRow(r.Context(), `select count(distinct user_id) from promocode_usages where promocode_id = any($1::bigint[])`, ids).Scan(&count)
	case "users":
		ids := uniquePositiveIDs(req.UserIDs)
		if len(ids) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "userIds are required"})
			return
		}
		_ = h.DB.QueryRow(r.Context(), `select count(distinct id) from users where deleted_at is null and status = 'active' and id = any($1::bigint[])`, ids).Scan(&count)
	default:
		_ = h.DB.QueryRow(r.Context(), `select count(*) from users where deleted_at is null and status = 'active'`).Scan(&count)
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.broadcast.schedule", "broadcast", nil, map[string]any{"audience": req.Audience, "count": count, "every": req.Every})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plannedRecipients": count, "status": "saved_for_manual_delivery"})
}

func promocodeIsActive(activeFrom, activeTo *time.Time, maxUses, usedCount int) bool {
	now := time.Now()
	if activeFrom != nil && now.Before(*activeFrom) {
		return false
	}
	if activeTo != nil && now.After(*activeTo) {
		return false
	}
	if maxUses > 0 && usedCount >= maxUses {
		return false
	}
	return true
}

func (h Handler) temporaryAdminURL(r *http.Request, code string) string {
	base := strings.Replace(promocodeApplyURL(r, code), "/access?promo=", "/promo-admin?promo=", 1)
	sep := "&"
	if !strings.Contains(base, "?") {
		sep = "?"
	}
	return base + sep + "key=" + h.promoAdminToken(code)
}

func (h Handler) promoAdminToken(code string) string {
	mac := hmac.New(sha256.New, []byte(h.promoAdminSigningSecret()))
	_, _ = mac.Write([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// S-NEW-2: promoAdminSigningSecret requires an explicit PromoAdminSecret.
// It used to fall back to OpenAIAPIKey/TelegramBotToken/a hardcoded value, which broke
// the "one key, one purpose" principle and leaked a hardcoded default.
// If the secret is not set, the signing function returns an empty string and
// promoAdminTokenValid returns false for any token, so access is closed.
func (h Handler) promoAdminSigningSecret() string {
	return strings.TrimSpace(h.PromoAdminSecret)
}

func (h Handler) promoAdminTokenValid(r *http.Request, code string) bool {
	// S-NEW-2: no secret means no valid tokens. Protects against the case where
	// PromoAdminSecret is empty and someone knows the hardcoded default.
	if h.promoAdminSigningSecret() == "" {
		return false
	}
	provided := strings.TrimSpace(r.URL.Query().Get("key"))
	if provided == "" {
		provided = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	expected := h.promoAdminToken(code)
	return provided != "" && hmac.Equal([]byte(provided), []byte(expected))
}

func temporaryAdminDefaultNote(purpose string) string {
	text := strings.TrimSpace(purpose)
	if text == "" {
		return "Временная админка по промокоду: смотреть срез аудитории, экспорт и доступные промпты резюмирования, пока промокод активен."
	}
	return text
}

func (h Handler) AdminSummaryPrompts(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "exports", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if writeNamedCached(w, "admin_summary_prompts", &h.c.adminSummaryPrompts) {
			return
		}
		rows, err := h.DB.Query(r.Context(), `select id, name, prompt, is_default, created_at from admin_summary_prompts order by is_default desc, id asc`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, prompt string
			var isDefault bool
			var createdAt time.Time
			if err := rows.Scan(&id, &name, &prompt, &isDefault, &createdAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			out = append(out, map[string]any{"id": id, "name": name, "prompt": prompt, "isDefault": isDefault, "createdAt": createdAt})
		}
		body, _ := json.Marshal(map[string]any{"ok": true, "prompts": out})
		h.c.adminSummaryPrompts.set(body, adminConfigCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req adminSummaryPromptRequest
		if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Prompt) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name and prompt are required"})
			return
		}
		var id int64
		err := h.DB.QueryRow(r.Context(), `insert into admin_summary_prompts (name, prompt, is_default, created_at, updated_at) values ($1, $2, $3, now(), now()) returning id`, strings.TrimSpace(req.Name), strings.TrimSpace(req.Prompt), req.IsDefault).Scan(&id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminSummaryPrompts.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.summary_prompt.create", "summary_prompt", &id, nil)
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "promptId": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) AdminSummaryPromptDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "exports", r.Method != http.MethodGet)
	if !ok {
		return
	}
	id, _, err := parseIDFromPath(r.URL.Path, "/api/admin/summary-prompts/")
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid prompt id"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req adminSummaryPromptRequest
		if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Prompt) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name and prompt are required"})
			return
		}
		_, err := h.DB.Exec(r.Context(), `update admin_summary_prompts set name=$2, prompt=$3, is_default=$4, updated_at=now() where id=$1`, id, strings.TrimSpace(req.Name), strings.TrimSpace(req.Prompt), req.IsDefault)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminSummaryPrompts.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.summary_prompt.patch", "summary_prompt", &id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		_, err := h.DB.Exec(r.Context(), `delete from admin_summary_prompts where id=$1 and is_default = false`, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.adminSummaryPrompts.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.summary_prompt.delete", "summary_prompt", &id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func parseAdminIDList(values []string, single string) []int64 {
	all := append([]string{}, values...)
	if single != "" {
		all = append(all, single)
	}
	out := []int64{}
	for _, raw := range all {
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && id > 0 {
				out = append(out, id)
			}
		}
	}
	return uniquePositiveIDs(out)
}
