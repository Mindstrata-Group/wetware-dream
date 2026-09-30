package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"

	"mindstrata-stage1/api/internal/config"
	"mindstrata-stage1/api/internal/db"
	"mindstrata-stage1/api/internal/httpapi"
)

// buildRelayTLSConfig adds the CA of our own relay proxy
// (infrastructure/relay-proxy, self-signed) on top of the system trust
// pool. Returns nil when RelayCACertPEM is empty: http.Transport then uses
// the usual system verification (compatible with the Deno Deploy proxy,
// which has a valid public certificate).
// The value is accepted either as raw PEM or as base64(PEM): PEM is
// multi-line, and in a .env file it is easier to keep as one base64 string.
func buildRelayTLSConfig(caCert string) *tls.Config {
	caCert = strings.TrimSpace(caCert)
	if caCert == "" {
		return nil
	}
	pemBytes := []byte(caCert)
	if !strings.HasPrefix(caCert, "-----BEGIN") {
		decoded, err := base64.StdEncoding.DecodeString(caCert)
		if err != nil {
			log.Fatalf("AI_RELAY_CA_CERT_PEM: не PEM и не декодируется как base64: %v", err)
		}
		pemBytes = decoded
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemBytes) {
		log.Fatal("AI_RELAY_CA_CERT_PEM задан, но не распарсился как PEM-сертификат")
	}
	return &tls.Config{RootCAs: pool}
}

func main() {
	cfg := config.Load()

	// K-NEW-5, K-NEW-6: fail fast on dangerous production configurations.
	if errs := cfg.Validate(); len(errs) > 0 {
		for _, e := range errs {
			log.Printf("CONFIG ERROR: %s", e)
		}
		log.Fatalf("config validation failed (%d errors) — отказ запуска", len(errs))
	}

	handler := httpapi.Handler{
		OpenAIBaseURL:            cfg.OpenAIBaseURL,
		PromoAdminSecret:         cfg.PromoAdminSecret,
		TelegramBotToken:         cfg.TelegramBotToken,
		TelegramChatID:           cfg.TelegramChatID,
		TelegramAPIBaseURL:       cfg.TelegramAPIBaseURL,
		TelegramBotUsername:      cfg.TelegramBotUsername,
		YooKassaShopID:           cfg.YooKassaShopID,
		YooKassaSecretKey:        cfg.YooKassaSecretKey,
		YooKassaWebhookPath:      cfg.YooKassaWebhookPath,
		GuestCookieSecret:        cfg.GuestCookieSecret,
		TrustedProxyIPs:          cfg.TrustedProxyIPs,
		MetricsBasicAuth:         cfg.MetricsBasicAuth,
		BootstrapAdminToken:      cfg.BootstrapAdminToken,
		AuthDevReturnResetToken:  cfg.AuthDevReturnResetToken,
		AuthDevReturnVerifyToken: cfg.AuthDevReturnVerifyToken,
		YandexClientID:           cfg.YandexClientID,
		YandexClientSecret:       cfg.YandexClientSecret,
		APIPublicBaseURL:         cfg.APIPublicBaseURL,
		MaxBotToken:              cfg.MaxBotToken,
		MaxBotUsername:           cfg.MaxBotUsername,
		MaxWebhookSecret:         cfg.MaxWebhookSecret,
		ModeHistory:              httpapi.NewGitModeHistoryStore(cfg.ModeHistoryRepoPath, cfg.ModeHistoryRepoURL, cfg.ModeHistoryDeployKey),
		GeminiAPIKey:             cfg.GeminiAPIKey,
		GeminiAPIBaseURL:         cfg.GeminiAPIBaseURL,
		GeminiVertexSAJSON:       cfg.GeminiVertexSAJSON,
		GeminiVertexProject:      cfg.GeminiVertexProject,
		GeminiVertexLocation:     cfg.GeminiVertexLocation,
		GeminiVertexBaseURL:      cfg.GeminiVertexBaseURL,
		AnthropicAPIKey:          cfg.AnthropicAPIKey,
		AnthropicAPIBaseURL:      cfg.AnthropicAPIBaseURL,
		DataProtectionProfile:    cfg.DataProtectionProfile,
		DataStorageCountry:       cfg.DataStorageCountry,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        512,
				MaxIdleConnsPerHost: 256,
				IdleConnTimeout:     90 * time.Second,
				TLSClientConfig:     buildRelayTLSConfig(cfg.RelayCACertPEM),
			},
		},
	}
	if cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pool, err := db.OpenPool(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("failed to open postgres: %v", err)
		}
		defer pool.Close()
		handler.DB = pool
		httpapi.RegisterDBPoolMetrics(pool)
	}

	// Initialise per-Handler caches (rate limiters, admin caches).
	httpapi.InitHandlerCaches(&handler)

	// Background delivery of deferred notifications (the broadcast's "second messenger").
	if handler.DB != nil {
		go handler.StartNotificationQueueWorker(context.Background())
	}

	router := httpapi.NewRouter(handler, cfg.CORSAllowedOrigins)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("API started on :%s (%s)", cfg.Port, cfg.AppEnv)
	log.Fatal(srv.ListenAndServe())
}
