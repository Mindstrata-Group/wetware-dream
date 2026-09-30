package config

import (
	"os"
	"strings"
)

type Config struct {
	Port               string
	AppEnv             string
	DatabaseURL        string
	CORSAllowedOrigins []string
	OpenAIBaseURL      string
	PromoAdminSecret   string
	TelegramBotToken   string
	TelegramChatID     string
	// TelegramAPIBaseURL: the Telegram Bot API base. api.telegram.org is blocked
	// from Russian servers, so in production this is set to the URL of a
	// proxy (e.g. Deno Deploy) that forwards to api.telegram.org.
	TelegramAPIBaseURL string
	// TelegramBotUsername: the bot username (Mindstrata_bot), for t.me deep links.
	TelegramBotUsername string
	YooKassaShopID      string
	YooKassaSecretKey   string
	// YooKassaWebhookPath: the path after /webhooks/yookassa/, e.g. "3JzPXAquX7tK".
	// Kept in env, not in code: otherwise any git push leaks the URL.
	YooKassaWebhookPath string
	// GuestCookieSecret: HMAC key for signing the guest cookie (L-1).
	GuestCookieSecret string
	// TrustedProxyIPs: comma-separated list of trusted proxy IPs/CIDRs (Caddy).
	// Only requests from these RemoteAddrs may use X-Forwarded-For.
	// Empty = trust everyone (legacy behaviour for dev/tests).
	TrustedProxyIPs          []string
	MetricsBasicAuth         string
	BootstrapAdminToken      string
	AuthDevReturnResetToken  bool
	AuthDevReturnVerifyToken bool
	YandexClientID           string
	YandexClientSecret       string
	APIPublicBaseURL         string
	// MaxBotToken: the Max Messenger bot token used to send notifications.
	MaxBotToken string
	// MaxBotUsername: the bot username in Max (e.g. "mindstrata_bot"), used in deep links.
	MaxBotUsername string
	// MaxWebhookSecret: random secret (32+ chars, [A-Za-z0-9_-]) that forms the
	// webhook path and is checked against the X-Max-Bot-Api-Secret header.
	// Empty or weak = the webhook route is not registered.
	MaxWebhookSecret string
	// ModeHistoryDeployKey: private SSH deploy key (PEM, ed25519) with write
	// access ONLY to the mode history repository, a git mirror of the edit
	// history of the modes' AI prompts. Empty = the feature is quietly off.
	ModeHistoryDeployKey string
	// ModeHistoryRepoPath: path on the container disk where the mode history
	// repository is cloned / lives.
	ModeHistoryRepoPath string
	// ModeHistoryRepoURL: SSH URL of the mode history repository. There is no
	// default: every installation has its own. Empty = feature off.
	ModeHistoryRepoURL string
	// GeminiAPIKey: key for the direct Google Gemini API (provider "gemini").
	// Empty = the provider is quietly off, routing goes through vsegpt.
	GeminiAPIKey string
	// GeminiAPIBaseURL: OpenAI-compatible Gemini API base. From a Russian
	// server generativelanguage.googleapis.com may be unreachable, so in
	// production this is set to the Deno proxy URL (see infrastructure/gemini-proxy).
	GeminiAPIBaseURL string
	// GeminiVertexSAJSON: Google service account key (raw JSON or base64).
	// The ONLY switch for Vertex AI mode: empty means AI Studio via
	// GeminiAPIBaseURL, set means Vertex. Needed because AI Studio rejects
	// by caller IP (400 "User location is not supported" for the relay's
	// datacenter address), while Vertex is addressed by region in the URL and
	// ignores the IP. Details and measurements are in ai_gemini_vertex.go.
	// ⚠️ Vertex is paid; it has no free tier.
	GeminiVertexSAJSON string
	// GeminiVertexProject: the Google Cloud project. Empty = project_id is taken
	// from the service account key itself.
	GeminiVertexProject string
	// GeminiVertexLocation: the Vertex region (us-central1 by default).
	GeminiVertexLocation string
	// GeminiVertexBaseURL: overrides the whole base. Required in production,
	// because *-aiplatform.googleapis.com is unreachable from Russia (measured: 000)
	// and traffic must go through the relay, which has its own address and secret path.
	GeminiVertexBaseURL string
	// AnthropicAPIKey: key for the direct Anthropic API (provider "anthropic").
	// Empty = the provider is quietly off.
	AnthropicAPIKey string
	// AnthropicAPIBaseURL: the Anthropic Messages API base; in production the Deno proxy
	// (see infrastructure/anthropic-proxy), since api.anthropic.com is unreachable from Russia.
	AnthropicAPIBaseURL string
	// RelayCACertPEM: PEM certificate of the self-signed CA of our own
	// relay proxy (see infrastructure/relay-proxy), a server with non-Russian egress
	// that *_API_BASE_URL can point to instead of Deno Deploy. Not a
	// secret (public certificate, no private key), but it is added
	// narrowly on top of the system trust pool, not instead of it: other
	// hosts (YooKassa, OAuth, etc.) are still verified against the usual CAs.
	RelayCACertPEM string
	// DataProtectionProfile: legal regime of the installation (none|ru|eu|us).
	// Set by the operator in the environment on purpose, not in the admin
	// panel. See docs/specs/data-protection.md.
	DataProtectionProfile string
	// DataStorageCountry: where the main database is hosted, declared by the
	// operator (ISO code). The RU profile warns in the admin panel unless it
	// is RU (152-FZ art. 18(5), data localisation).
	DataStorageCountry string
}

func Load() Config {
	return Config{
		Port:        getEnv("PORT", "18080"),
		AppEnv:      getEnv("APP_ENV", "local"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		CORSAllowedOrigins: splitCSV(
			getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
		),
		// OPENAI_API_KEY is no longer read (2026-09-30): the vsegpt key lives
		// only in the gateway registry in the database (admin -> Orchestration -> AI).
		// The env key once leaked into the repository and kept working in production
		// for half a year; there must be no second source that survives a rotation in the admin UI.
		OpenAIBaseURL:            getEnv("OPENAI_BASE_URL", "https://api.vsegpt.ru/v1"),
		PromoAdminSecret:         os.Getenv("PROMO_ADMIN_SECRET"),
		TelegramBotToken:         os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:           os.Getenv("TELEGRAM_CHAT_ID"),
		TelegramAPIBaseURL:       getEnv("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
		TelegramBotUsername:      os.Getenv("TELEGRAM_BOT_USERNAME"),
		YooKassaShopID:           os.Getenv("YOOKASSA_SHOP_ID"),
		YooKassaSecretKey:        os.Getenv("YOOKASSA_SECRET_KEY"),
		YooKassaWebhookPath:      os.Getenv("YOOKASSA_WEBHOOK_PATH"),
		GuestCookieSecret:        os.Getenv("GUEST_COOKIE_SECRET"),
		TrustedProxyIPs:          splitCSV(os.Getenv("TRUSTED_PROXY_IPS")),
		MetricsBasicAuth:         os.Getenv("METRICS_BASIC_AUTH"),
		BootstrapAdminToken:      os.Getenv("BOOTSTRAP_ADMIN_TOKEN"),
		AuthDevReturnResetToken:  strings.EqualFold(os.Getenv("AUTH_DEV_RETURN_RESET_TOKEN"), "true"),
		AuthDevReturnVerifyToken: strings.EqualFold(os.Getenv("AUTH_DEV_RETURN_VERIFY_TOKEN"), "true"),
		YandexClientID:           os.Getenv("YANDEX_CLIENT_ID"),
		YandexClientSecret:       os.Getenv("YANDEX_CLIENT_SECRET"),
		APIPublicBaseURL:         os.Getenv("API_PUBLIC_BASE_URL"),
		MaxBotToken:              os.Getenv("MAX_BOT_TOKEN"),
		MaxBotUsername:           os.Getenv("MAX_BOT_USERNAME"),
		MaxWebhookSecret:         os.Getenv("MAX_WEBHOOK_SECRET"),
		ModeHistoryDeployKey:     os.Getenv("MODE_HISTORY_DEPLOY_KEY"),
		ModeHistoryRepoPath:      getEnv("MODE_HISTORY_REPO_PATH", "/data/mode-history"),
		ModeHistoryRepoURL:       os.Getenv("MODE_HISTORY_REPO_URL"),
		GeminiAPIKey:             os.Getenv("GEMINI_API_KEY"),
		GeminiAPIBaseURL:         getEnv("GEMINI_API_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai"),
		GeminiVertexSAJSON:       os.Getenv("GEMINI_VERTEX_SA_JSON"),
		GeminiVertexProject:      os.Getenv("GEMINI_VERTEX_PROJECT"),
		GeminiVertexLocation:     os.Getenv("GEMINI_VERTEX_LOCATION"),
		GeminiVertexBaseURL:      os.Getenv("GEMINI_VERTEX_BASE_URL"),
		AnthropicAPIKey:          os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicAPIBaseURL:      getEnv("ANTHROPIC_API_BASE_URL", "https://api.anthropic.com"),
		RelayCACertPEM:           os.Getenv("AI_RELAY_CA_CERT_PEM"),
		DataProtectionProfile:    getEnv("DATA_PROTECTION_PROFILE", "none"),
		DataStorageCountry:       os.Getenv("DATA_STORAGE_COUNTRY"),
	}
}

// Validate checks security invariants when APP_ENV=production.
// Returns a list of errors (empty if all is OK). main.go must fail fatally
// if the list is non-empty.
func (c Config) Validate() []string {
	if c.AppEnv != "production" {
		return nil
	}
	var errs []string
	if strings.TrimSpace(c.GuestCookieSecret) == "" {
		errs = append(errs, "GUEST_COOKIE_SECRET обязателен в production (L-1 HMAC guest cookie)")
	}
	// K-NEW-6: forbid dev flags that return reset/verification tokens in the response.
	if c.AuthDevReturnResetToken {
		errs = append(errs, "AUTH_DEV_RETURN_RESET_TOKEN=true запрещён в production (account takeover risk)")
	}
	if c.AuthDevReturnVerifyToken {
		errs = append(errs, "AUTH_DEV_RETURN_VERIFY_TOKEN=true запрещён в production")
	}
	// Rate limits key on the client IP; with an empty list X-Forwarded-For is
	// trusted from anyone and a forged header resets every limit.
	if len(c.TrustedProxyIPs) == 0 {
		errs = append(errs, "TRUSTED_PROXY_IPS обязателен в production (адреса обратного прокси; без него X-Forwarded-For подделывается)")
	}
	return errs
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
