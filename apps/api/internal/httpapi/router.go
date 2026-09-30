package httpapi

import (
	"log"
	"net/http"
	"strings"

	"mindstrata-stage1/api/internal/modules/argumentclinic"
)

func NewRouter(h Handler, corsOrigins []string) http.Handler {
	if h.c == nil {
		h.c = newHandlerCaches()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/health/deep", h.HealthDeep)
	mux.HandleFunc("/health/frontend-errors", h.FrontendErrorsAlertCheck)
	mux.HandleFunc("/health/ai-errors", h.AIErrorsAlertCheck)
	mux.HandleFunc("/db-check", h.DBCheck)
	mux.HandleFunc("/metrics", h.Metrics)
	mux.HandleFunc("/api/_error", h.FrontendErrorReport)
	mux.HandleFunc("/api/debug/client-log", h.DebugClientLog)

	mux.HandleFunc("/api/public/demo-modes", h.PublicDemoModes)
	mux.HandleFunc("/api/public/site-content", h.PublicSiteContent)
	mux.HandleFunc("/api/public/site-media/", h.PublicSiteMedia)
	mux.HandleFunc("/api/public/tariffs", h.PublicTariffs)
	mux.HandleFunc("/api/public/analytics", h.PublicAnalytics)
	mux.HandleFunc("/api/cookie-consent", h.CookieConsent)
	mux.HandleFunc("/api/argument-clinic/vote", argumentclinic.Module{DB: h.DB}.Vote)

	mux.HandleFunc("/api/auth/login", h.AuthLogin)
	mux.HandleFunc("/api/auth/logout", h.AuthLogout)
	mux.HandleFunc("/api/auth/me", h.AuthMe)
	mux.HandleFunc("/api/auth/forgot-password", h.AuthForgotPassword)
	mux.HandleFunc("/api/auth/reset-password", h.AuthResetPassword)
	mux.HandleFunc("/api/auth/register", h.AuthRegister)
	mux.HandleFunc("/api/auth/verify-email", h.AuthVerifyEmail)
	mux.HandleFunc("/api/auth/oauth/providers", h.AuthOAuthProviders)
	mux.HandleFunc("/api/auth/oauth/", h.AuthOAuth)
	mux.HandleFunc("/api/profile", h.Profile)
	mux.HandleFunc("/api/profile/settings", h.ProfileSettings)
	mux.HandleFunc("/api/profile/export", h.ProfileExport)
	mux.HandleFunc("/api/privacy/consents", h.PrivacyConsents)
	mux.HandleFunc("/api/privacy/consents/withdraw", h.PrivacyConsentsWithdraw)
	mux.HandleFunc("/api/notifications/inbox", h.NotificationsInbox)
	mux.HandleFunc("/api/notifications/preferences", h.NotificationPreferences)
	mux.HandleFunc("/api/notifications/max/start-link", h.MaxNotificationsStartLink)
	mux.HandleFunc("/api/notifications/max/unlink", h.MaxNotificationsUnlink)
	mux.HandleFunc("/api/notifications/telegram/start-link", h.TelegramNotificationsStartLink)
	mux.HandleFunc("/api/notifications/telegram/unlink", h.TelegramNotificationsUnlink)
	mux.HandleFunc("/api/billing/autorenew/disable", h.DisableYooKassaAutoRenew)
	mux.HandleFunc("/api/bootstrap/admin", h.BootstrapAdmin)

	mux.HandleFunc("/api/access/status", h.AccessStatus)
	mux.HandleFunc("/api/access/promocode/apply", h.ApplyPromocode)
	mux.HandleFunc("/api/promo/validate", h.ApplyPromocode)
	mux.HandleFunc("/api/promo-admin/status", h.PromoAdminStatus)
	mux.HandleFunc("/api/promo-admin/summarize", h.PromoAdminSummarize)

	mux.HandleFunc("/api/chat/start", h.StartChat)
	mux.HandleFunc("/api/chat/attachments", h.ChatAttachmentUpload)
	mux.HandleFunc("/api/chat/select-mode", h.SelectMode)
	mux.HandleFunc("/api/chat/send", h.SendMessage)
	mux.HandleFunc("/api/chat/history", h.GetHistory)
	mux.HandleFunc("/api/chat/complete", h.CompleteDialog)

	// The "Department N" training game (IRIT-RTF practicum): public routes without
	// authorisation, see game_results.go.
	mux.HandleFunc("/api/game/result", h.GameResult)
	mux.HandleFunc("/api/game/stats", h.GameStats)
	mux.HandleFunc("/api/game/results", h.GameResults)
	mux.HandleFunc("/api/game/tasks", h.GameTasks)
	mux.HandleFunc("/api/game/session", h.GameSession)
	// Command-based service management (MCP agents), see mcp_admin.go.
	mux.HandleFunc("/api/mcp/call", h.MCPAdmin)

	mux.HandleFunc("/api/admin/status", h.AdminSystemStatus)
	mux.HandleFunc("/api/admin/stats", h.AdminStats)
	mux.HandleFunc("/api/admin/users", h.AdminUsers)
	mux.HandleFunc("/api/admin/users/", h.AdminUserDetail)
	mux.HandleFunc("/api/admin/access", h.AdminAccess)
	mux.HandleFunc("/api/admin/modes", h.AdminModes)
	mux.HandleFunc("/api/admin/modes/guardrail", h.AdminModeGuardrail)
	mux.HandleFunc("/api/admin/modes/model-stats", h.WithAdminAuth("modes", false).WithCache(&h.c.adminModeModelStats, adminConfigCacheTTL).Handler(h.AdminModeModelStats))
	mux.HandleFunc("/api/admin/ai-models", h.AdminAIModels)
	mux.HandleFunc("/api/admin/modes/", h.AdminModeDetail)
	mux.HandleFunc("/api/admin/tariffs", h.AdminTariffs)
	mux.HandleFunc("/api/admin/tariffs/", h.AdminTariffDetail)
	mux.HandleFunc("/api/admin/tariff-groups", h.AdminTariffGroups)
	mux.HandleFunc("/api/admin/tariff-groups/", h.AdminTariffGroupDetail)
	mux.HandleFunc("/api/admin/promocodes", h.AdminPromocodes)
	mux.HandleFunc("/api/admin/promocodes/bulk-deactivate", h.AdminPromocodesBulkDeactivate)
	mux.HandleFunc("/api/admin/orchestration-prompt", h.AdminOrchestrationPrompt)
	mux.HandleFunc("/api/admin/dialog-summary-prompt", h.AdminDialogSummaryPrompt)
	mux.HandleFunc("/api/admin/lead-summary-prompt", h.AdminLeadSummaryPrompt)
	mux.HandleFunc("/api/admin/ai-settings", h.AdminAISettings)
	mux.HandleFunc("/api/admin/ai-gateways", h.AdminAIGateways)
	mux.HandleFunc("/api/admin/ai-gateways/", h.AdminAIGatewayDetail)
	mux.HandleFunc("/api/admin/analytics-settings", h.AdminAnalyticsSettings)
	mux.HandleFunc("/api/admin/site-content", h.AdminSiteContentDispatch)
	mux.HandleFunc("/api/admin/site-content/flush", h.AdminSiteContentFlush)
	mux.HandleFunc("/api/admin/site-media", h.AdminSiteMediaUpload)
	mux.HandleFunc("/api/admin/notifications/templates", h.AdminNotificationTemplates)
	mux.HandleFunc("/api/admin/notifications/test", h.AdminNotificationTest)
	mux.HandleFunc("/api/admin/notifications/send", h.AdminNotificationSend)
	mux.HandleFunc("/api/admin/notifications/preview", h.AdminNotificationPreview)
	mux.HandleFunc("/api/admin/notifications/history", h.AdminNotificationHistory)
	mux.HandleFunc("/api/admin/promocodes/", h.AdminPromocodeDetail)
	mux.HandleFunc("/api/admin/exports/messages", h.AdminExports)
	mux.HandleFunc("/api/admin/broadcasts", h.AdminBroadcasts)
	mux.HandleFunc("/api/admin/summary-prompts", h.AdminSummaryPrompts)
	mux.HandleFunc("/api/admin/summary-prompts/", h.AdminSummaryPromptDetail)
	mux.HandleFunc("/api/admin/dialogs", h.AdminDialogs)
	mux.HandleFunc("/api/admin/dialogs/", h.AdminDialogDetail)
	mux.HandleFunc("/api/admin/payments", h.AdminPayments)
	mux.HandleFunc("/api/admin/payments/access-recovery", h.AdminAccessRecovery)
	mux.HandleFunc("/api/admin/payments/yookassa/config", h.AdminYooKassaConfig)
	mux.HandleFunc("/api/admin/payments/yookassa/create", h.AdminYooKassaCreatePayment)
	mux.HandleFunc("/api/admin/payments/subscriptions/revoke", h.AdminRevokeSubscriptionAccess)
	mux.HandleFunc("/api/admin/payments/yookassa/test-charge", h.AdminYooKassaTestAutoCharge)
	mux.HandleFunc("/api/admin/payments/yookassa/renewals/run", h.AdminYooKassaRunDueRenewals)
	mux.HandleFunc("/api/admin/payments/yookassa/run-due-renewals", h.AdminYooKassaRunDueRenewals)
	mux.HandleFunc("/api/admin/client-logs", h.AdminClientLogs)
	mux.HandleFunc("/api/admin/data-protection", h.AdminDataProtection)
	mux.HandleFunc("/api/admin/data-protection/gateway-country", h.AdminDataProtectionGatewayCountry)
	mux.HandleFunc("/api/admin/data-protection/access-log", h.AdminDataProtectionAccessLog)
	mux.HandleFunc("/api/payments/yookassa/create", h.PublicYooKassaCreatePayment)
	mux.HandleFunc("/api/payments/yookassa/autorenew/disable", h.DisableYooKassaAutoRenew)

	// The YooKassa webhook is registered only if YOOKASSA_WEBHOOK_PATH is set;
	// otherwise the path is not published (security by obscurity for the unmodified path,
	// the IP allowlist stays on the YooKassa side).
	if path := strings.TrimSpace(h.YooKassaWebhookPath); path != "" {
		mux.HandleFunc("/webhooks/yookassa/"+strings.TrimPrefix(path, "/"), h.YooKassaWebhook)
	}

	// Max Messenger webhook: accepts updates from the bot. The path is a random
	// secret unrelated to the bot token (R-035: the old path was the first 20
	// token characters and leaked into access logs); the same secret must come
	// in the X-Max-Bot-Api-Secret header, which Max sends for subscriptions
	// registered with it. Without a strong secret the route does not exist.
	if strings.TrimSpace(h.MaxBotToken) != "" && validMaxWebhookSecret(h.MaxWebhookSecret) {
		secret := h.MaxWebhookSecret
		mux.HandleFunc("/webhooks/max/"+secret, func(w http.ResponseWriter, r *http.Request) {
			h.MaxWebhook(w, r, secret)
		})
	} else if strings.TrimSpace(h.MaxBotToken) != "" {
		log.Printf("max webhook disabled: MAX_WEBHOOK_SECRET is missing or weak (need 32+ chars of [A-Za-z0-9_-])")
	}

	mux.HandleFunc("/api/tester/users/", h.TesterUserDetail)
	mux.HandleFunc("/api/tester/users", h.TesterUsers)
	mux.HandleFunc("/api/tester/status", h.TesterStatus)
	mux.HandleFunc("/api/tester/run-check", h.TesterRunCheck)
	mux.HandleFunc("/api/tester/user-chat-preview", h.TesterUserChatPreview)
	mux.HandleFunc("/api/expert/status", h.ExpertStatus)

	allowed := make(map[string]struct{}, len(corsOrigins))
	for _, origin := range corsOrigins {
		allowed[origin] = struct{}{}
	}
	return withRequestID(withPrometheusMetrics(withCORS(mux, allowed)))
}
