package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type oauthProviderConfig struct {
	Provider      string
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	AuthEndpoint  string
	TokenEndpoint string
	Scopes        []string
}

func (h Handler) oauthProviderConfigured(provider string) bool {
	switch provider {
	case "yandex":
		return strings.TrimSpace(h.YandexClientID) != "" && strings.TrimSpace(h.YandexClientSecret) != ""
	default:
		return false
	}
}

func (h Handler) oauthConfig(provider string) (oauthProviderConfig, error) {
	base := h.apiPublicBaseURL()
	switch provider {
	case "yandex":
		cfg := oauthProviderConfig{
			Provider:      "yandex",
			ClientID:      strings.TrimSpace(h.YandexClientID),
			ClientSecret:  strings.TrimSpace(h.YandexClientSecret),
			RedirectURI:   base + "/api/auth/oauth/yandex/callback",
			AuthEndpoint:  "https://oauth.yandex.ru/authorize",
			TokenEndpoint: "https://oauth.yandex.ru/token",
			Scopes:        []string{"login:email", "login:info", "login:default_phone"},
		}
		if cfg.ClientID == "" || cfg.ClientSecret == "" {
			return cfg, errors.New("yandex oauth is not configured")
		}
		return cfg, nil
	default:
		return oauthProviderConfig{}, errors.New("unknown oauth provider")
	}
}

func (c oauthProviderConfig) AuthURL(state string, forceAccount bool) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURI)
	q.Set("state", state)
	if len(c.Scopes) > 0 {
		q.Set("scope", strings.Join(c.Scopes, " "))
	}
	if forceAccount {
		q.Set("force_confirm", "yes")
	}
	return c.AuthEndpoint + "?" + q.Encode()
}

func (h Handler) oauthClient() *http.Client {
	if h.HTTPClient != nil {
		return h.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (h Handler) exchangeOAuthProfile(ctx context.Context, provider, code string) (OAuthProfile, error) {
	cfg, err := h.oauthConfig(provider)
	if err != nil {
		return OAuthProfile{}, err
	}
	switch provider {
	case "yandex":
		return h.exchangeYandexProfile(ctx, cfg, code)
	default:
		return OAuthProfile{}, errors.New("unknown provider")
	}
}

func (h Handler) exchangeYandexProfile(ctx context.Context, cfg oauthProviderConfig, code string) (OAuthProfile, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("redirect_uri", cfg.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthProfile{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.oauthClient().Do(req)
	if err != nil {
		return OAuthProfile{}, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return OAuthProfile{}, oauthHTTPError("yandex", resp.StatusCode, string(payload))
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(payload, &token); err != nil || token.AccessToken == "" {
		return OAuthProfile{}, errors.New("invalid yandex token response")
	}

	profileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://login.yandex.ru/info?format=json", nil)
	if err != nil {
		return OAuthProfile{}, err
	}
	profileReq.Header.Set("Authorization", "OAuth "+token.AccessToken)
	profileResp, err := h.oauthClient().Do(profileReq)
	if err != nil {
		return OAuthProfile{}, err
	}
	defer profileResp.Body.Close()
	profilePayload, _ := io.ReadAll(io.LimitReader(profileResp.Body, 1<<20))
	if profileResp.StatusCode >= 300 {
		return OAuthProfile{}, oauthHTTPError("yandex-profile", profileResp.StatusCode, string(profilePayload))
	}

	var raw map[string]any
	_ = json.Unmarshal(profilePayload, &raw)
	var y struct {
		ID           string   `json:"id"`
		Login        string   `json:"login"`
		DisplayName  string   `json:"display_name"`
		RealName     string   `json:"real_name"`
		DefaultEmail string   `json:"default_email"`
		Emails       []string `json:"emails"`
		DefaultPhone struct {
			Number string `json:"number"`
		} `json:"default_phone"`
		DefaultAvatarID string `json:"default_avatar_id"`
		IsAvatarEmpty   bool   `json:"is_avatar_empty"`
	}
	if err := json.Unmarshal(profilePayload, &y); err != nil || y.ID == "" {
		return OAuthProfile{}, errors.New("invalid yandex profile response")
	}
	email := normalizeEmail(y.DefaultEmail)
	if email == "" && len(y.Emails) > 0 {
		email = normalizeEmail(y.Emails[0])
	}
	name := strings.TrimSpace(y.DisplayName)
	if name == "" {
		name = strings.TrimSpace(y.RealName)
	}
	if name == "" {
		name = strings.TrimSpace(y.Login)
	}
	avatar := ""
	if !y.IsAvatarEmpty && strings.TrimSpace(y.DefaultAvatarID) != "" {
		avatar = "https://avatars.yandex.net/get-yapic/" + strings.TrimSpace(y.DefaultAvatarID) + "/islands-200"
	}
	phone := normalizePhone(y.DefaultPhone.Number)
	return OAuthProfile{Provider: "yandex", ProviderUserID: y.ID, Email: email, EmailVerified: email != "", Phone: phone, PhoneVerified: phone != "", DisplayName: name, AvatarURL: avatar, Raw: raw}, nil
}
