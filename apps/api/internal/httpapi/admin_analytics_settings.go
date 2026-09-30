package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Web analytics settings (Yandex.Metrica) live in system_settings,
// so the counter can be changed from the admin UI without a deploy.
const (
	settingMetrikaCounterID = "analytics_metrika_counter_id"
	settingMetrikaParams    = "analytics_metrika_params"

	publicAnalyticsCacheTTL = 5 * time.Minute
	metrikaParamsMaxBytes   = 4 << 10
)

// Default counter init parameters: rendered in the admin UI as a hint
// and used by the frontend when the admin has not set their own.
const defaultMetrikaParams = `{"clickmap":true,"trackLinks":true,"accurateTrackBounce":true,"webvisor":false}`

func validMetrikaCounterID(id string) bool {
	if id == "" {
		return true // empty value = counter disabled
	}
	if len(id) > 12 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validMetrikaParams(params string) bool {
	if params == "" {
		return true
	}
	if len(params) > metrikaParamsMaxBytes {
		return false
	}
	// JSON object only: the counter is initialised as ym(id, 'init', {...}),
	// so an arbitrary script cannot be injected here.
	var obj map[string]any
	return json.Unmarshal([]byte(params), &obj) == nil
}

func (h Handler) analyticsSettings(ctx context.Context) (counterID, params string) {
	counterID, _ = h.systemSetting(ctx, settingMetrikaCounterID)
	params, _ = h.systemSetting(ctx, settingMetrikaParams)
	if params == "" {
		params = defaultMetrikaParams
	}
	return counterID, params
}

// AdminAnalyticsSettings handles GET/POST /api/admin/analytics-settings.
// Manages the Yandex.Metrica counter from the "System" tab.
func (h Handler) AdminAnalyticsSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "system", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if writeNamedCached(w, "admin_analytics_settings", &h.c.adminAnalyticsSettings) {
			return
		}
		counterID, params := h.analyticsSettings(r.Context())
		body, _ := json.Marshal(map[string]any{
			"ok":               true,
			"metrikaCounterId": counterID,
			"metrikaParams":    params,
		})
		h.c.adminAnalyticsSettings.set(body, adminConfigCacheTTL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	case http.MethodPost:
		var req struct {
			MetrikaCounterID string `json:"metrikaCounterId"`
			MetrikaParams    string `json:"metrikaParams"`
		}
		if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		counterID := strings.TrimSpace(req.MetrikaCounterID)
		params := strings.TrimSpace(req.MetrikaParams)
		if !validMetrikaCounterID(counterID) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "номер счётчика — только цифры (или пусто, чтобы выключить)"})
			return
		}
		if !validMetrikaParams(params) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "параметры должны быть JSON-объектом"})
			return
		}
		settings := map[string]string{
			settingMetrikaCounterID: counterID,
			settingMetrikaParams:    params,
		}
		for key, value := range settings {
			_, err := h.DB.Exec(r.Context(), `
				insert into system_settings (key, value, description, created_at, updated_at)
				values ($1, $2, 'Web analytics setting', now(), now())
				on conflict (key) do update set value=excluded.value, updated_at=now()`, key, value)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
		h.c.adminAnalyticsSettings.clear()
		h.c.publicAnalytics.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.analytics_settings", "system_setting", nil, map[string]any{
			"metrikaCounterId": counterID,
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

// PublicAnalytics handles GET /api/public/analytics. Returns the Metrica
// counter number and init parameters to the frontend; an empty counterId means analytics is off.
func (h Handler) PublicAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	if writeNamedCached(w, "public_analytics", &h.c.publicAnalytics) {
		return
	}
	counterID, params := h.analyticsSettings(r.Context())
	body, _ := json.Marshal(map[string]any{
		"ok":               true,
		"metrikaCounterId": counterID,
		"metrikaParams":    json.RawMessage(params),
	})
	h.c.publicAnalytics.set(body, publicAnalyticsCacheTTL)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
