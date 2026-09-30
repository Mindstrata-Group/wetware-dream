package httpapi

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
)

const (
	yooKassaTestModeSetting   = "yookassa_test_mode_enabled"
	yooKassaTestShopIDSetting = "yookassa_test_shop_id"
	yooKassaTestSecretSetting = "yookassa_test_secret_key"
)

type yooKassaCredentials struct {
	ShopID    string
	SecretKey string
	Mode      string
}

func (c yooKassaCredentials) configured() bool {
	return strings.TrimSpace(c.ShopID) != "" && strings.TrimSpace(c.SecretKey) != ""
}

func (c yooKassaCredentials) basicAuth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.ShopID+":"+c.SecretKey))
}

func (h Handler) activeYooKassaCredentials(ctx context.Context) (yooKassaCredentials, error) {
	live := yooKassaCredentials{
		ShopID:    strings.TrimSpace(h.YooKassaShopID),
		SecretKey: strings.TrimSpace(h.YooKassaSecretKey),
		Mode:      "live",
	}
	enabled, err := h.systemSetting(ctx, yooKassaTestModeSetting)
	if err != nil {
		return live, err
	}
	if !settingEnabled(enabled) {
		return live, nil
	}
	testShopID, err := h.systemSetting(ctx, yooKassaTestShopIDSetting)
	if err != nil {
		return live, err
	}
	testSecret, err := h.systemSetting(ctx, yooKassaTestSecretSetting)
	if err != nil {
		return live, err
	}
	return yooKassaCredentials{
		ShopID:    strings.TrimSpace(testShopID),
		SecretKey: strings.TrimSpace(testSecret),
		Mode:      "test",
	}, nil
}

func settingEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

type adminYooKassaConfigRequest struct {
	TestModeEnabled *bool   `json:"testModeEnabled"`
	TestShopID      *string `json:"testShopId"`
	TestSecretKey   *string `json:"testSecretKey"`
	ClearTestSecret bool    `json:"clearTestSecret"`
}

func (h Handler) AdminYooKassaConfig(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireRole(w, r, "owner", "admin")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.writeAdminYooKassaConfig(w, r)
	case http.MethodPost:
		var req adminYooKassaConfigRequest
		if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		settings := map[string]string{}
		if req.TestModeEnabled != nil {
			if *req.TestModeEnabled {
				settings[yooKassaTestModeSetting] = "1"
			} else {
				settings[yooKassaTestModeSetting] = "0"
			}
		}
		if req.TestShopID != nil {
			settings[yooKassaTestShopIDSetting] = strings.TrimSpace(*req.TestShopID)
		}
		if req.ClearTestSecret {
			settings[yooKassaTestSecretSetting] = ""
		} else if req.TestSecretKey != nil && strings.TrimSpace(*req.TestSecretKey) != "" {
			settings[yooKassaTestSecretSetting] = strings.TrimSpace(*req.TestSecretKey)
		}
		for key, value := range settings {
			if _, err := h.DB.Exec(r.Context(), `
				insert into system_settings (key, value, description, created_at, updated_at)
				values ($1, $2, 'YooKassa runtime payment setting', now(), now())
				on conflict (key) do update set value=excluded.value, updated_at=now()`, key, value); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.yookassa_config", "system_setting", nil, map[string]any{
			"testModeChanged": req.TestModeEnabled != nil,
			"testShopChanged": req.TestShopID != nil,
			"testSecretSet":   req.TestSecretKey != nil && strings.TrimSpace(*req.TestSecretKey) != "",
			"testSecretClear": req.ClearTestSecret,
		})
		h.writeAdminYooKassaConfig(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) writeAdminYooKassaConfig(w http.ResponseWriter, r *http.Request) {
	testMode, err := h.systemSetting(r.Context(), yooKassaTestModeSetting)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	testShopID, err := h.systemSetting(r.Context(), yooKassaTestShopIDSetting)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	testSecret, err := h.systemSetting(r.Context(), yooKassaTestSecretSetting)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	active, err := h.activeYooKassaCredentials(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	liveShop := strings.TrimSpace(h.YooKassaShopID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"testModeEnabled":     settingEnabled(testMode),
		"activeMode":          active.Mode,
		"activeConfigured":    active.configured(),
		"liveConfigured":      liveShop != "" && strings.TrimSpace(h.YooKassaSecretKey) != "",
		"liveShopIdMasked":    maskSecret(liveShop),
		"testConfigured":      strings.TrimSpace(testShopID) != "" && strings.TrimSpace(testSecret) != "",
		"testShopId":          strings.TrimSpace(testShopID),
		"testSecretKeyMasked": maskSecret(testSecret),
	})
}
