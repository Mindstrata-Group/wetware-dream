package archtest

import (
	"path"
	"strings"
)

// tableOwner assigns every table to the module that owns it. A new migration
// that creates a table must add it here (TestTableOwnership_EveryTableHasOwner).
var tableOwner = map[string]string{
	// kernel: installation-wide settings and schema bookkeeping.
	"system_settings": "kernel",
	"alembic_version": "kernel",

	// auth: identities, sessions, account security.
	"users":                     "auth",
	"auth_sessions":             "auth",
	"oauth_states":              "auth",
	"user_identities":           "auth",
	"password_reset_tokens":     "auth",
	"email_verification_tokens": "auth",
	"jwt_token_usage":           "auth",
	"security_events":           "auth",
	"cookie_consents":           "auth",

	// access: tariffs, promo codes, quotas, per-mode access.
	"tariffs":                     "access",
	"tariff_groups":               "access",
	"tariff_mode":                 "access",
	"user_mode_access":            "access",
	"promocodes":                  "access",
	"promocode_targets":           "access",
	"promocode_usages":            "access",
	"daily_message_counts":        "access",
	"daily_mode_usage":            "access",
	"message_usage":               "access",
	"dialog_message_access_usage": "access",
	"admin_mode_usage_resets":     "access",

	// billing: payments and subscriptions (YooKassa).
	"invoices":                   "billing",
	"payment_methods":            "billing",
	"subscriptions":              "billing",
	"autopay_subscriptions":      "billing",
	"billing_runs":               "billing",
	"billing_run_items":          "billing",
	"recurring_payment_attempts": "billing",
	"yookassa_webhook_events":    "billing",
	"log_income":                 "billing",

	// chat: dialogs, messages, attachments, orchestration.
	"dialogs_messages":            "chat",
	"users_dialogs":               "chat",
	"chat_files":                  "chat",
	"chat_file_blobs":             "chat",
	"chat_message_attachments":    "chat",
	"orchestration_decision_logs": "chat",
	"message_terminologies":       "chat",
	"group_chats":                 "chat",

	// modes: psychological modes, prompts, summaries.
	"modes":                   "modes",
	"prompts":                 "modes",
	"public_demo_modes_cache": "modes",
	"admin_summary_prompts":   "modes",
	"mode_reminder_logs":      "modes",
	"mode_usage_reminders":    "modes",

	// aigateway: AI provider registry and provider telemetry.
	"ai_gateways":        "aigateway",
	"ai_provider_events": "aigateway",

	// notifications: channels, queue, inbox, reminders, broadcasts.
	"notification_channel_consents":  "notifications",
	"notification_channel_queue":     "notifications",
	"notification_consent_bonuses":   "notifications",
	"notification_contacts":          "notifications",
	"notification_delivery_attempts": "notifications",
	"notification_inbox":             "notifications",
	"notification_reachability":      "notifications",
	"notification_templates":         "notifications",
	"push_subscriptions":             "notifications",
	"reminder_configs":               "notifications",
	"reminder_logs":                  "notifications",
	"reminder_message_logs":          "notifications",
	"user_inactivity_reminders":      "notifications",
	"admin_notification_sends":       "notifications",

	// sitecontent: CMS texts and media of the public site.
	"site_content": "sitecontent",
	"site_media":   "sitecontent",

	// games: the teaching game backend (tir.* static site lives elsewhere).
	"game_results":  "games",
	"game_sessions": "games",
	"game_tasks":    "games",

	// argumentclinic: the public vote widget (pilot module).
	"argument_clinic_votes": "argumentclinic",

	// admin: back-office audit, exports, MCP machine registry.
	"admin_audit_log":        "admin",
	"admin_export_summaries": "admin",
	"tunnel_machines":        "admin",

	// privacy: data-protection profile, consent journal, data export/erasure
	// (docs/specs/data-protection.md).
	"consent_records": "privacy",
	// Reversible pseudonymisation vault (internal/pseudonym).
	"pii_vault": "privacy",
}

// httpapiFileModule maps files of the legacy internal/httpapi package to the
// module they will move to. Rules are checked in order; the first matching
// prefix wins. Every non-test file must match
// (TestFileModule_EveryLegacyFileAssigned).
var httpapiFileModule = []struct{ prefix, module string }{
	// Grab-bag files first: exact names before the broad prefixes below.
	{"admin_data_protection.go", "privacy"},
	{"data_protection", "privacy"},
	{"privacy_", "privacy"},
	{"profile_export.go", "privacy"},
	{"handlers.go", "chat"},         // Handler struct + StartChat/SelectMode; to be split
	{"handler_caches.go", "kernel"}, // per-Handler caches; split per module on move
	{"admin_cache_simple.go", "kernel"},
	{"admin_middleware_chain.go", "kernel"},
	{"admin_access_recovery.go", "billing"},
	{"admin_payments_handlers.go", "billing"},
	{"admin_ai_", "aigateway"},
	{"admin_mode_", "modes"},
	{"admin_promocode_", "access"},
	{"admin_access_handlers.go", "access"},
	{"admin_tariff_handlers.go", "access"},
	{"admin_extra_tariff", "access"},
	{"admin_broadcast_summary.go", "notifications"},
	{"admin_", "admin"},
	{"mcp_", "admin"},
	{"tester_", "admin"},
	{"expert_handlers.go", "admin"},

	{"auth_", "auth"},
	{"oauth_", "auth"},
	{"cookie_consent.go", "auth"},
	{"bootstrap_handlers.go", "auth"},

	{"access", "access"}, // access.go, access_promocode_helpers.go
	{"promo_admin_handlers.go", "access"},
	{"public_tariffs.go", "access"},
	{"chat_quota_", "access"},

	{"yookassa_", "billing"},
	{"billing_", "billing"},

	{"ai_", "aigateway"},

	{"chat_modes.go", "modes"},
	{"mode_history.go", "modes"},
	{"public.go", "modes"}, // public demo modes
	{"chat_", "chat"},

	{"notification", "notifications"},
	{"lead_notifications.go", "notifications"},
	{"telegram", "notifications"},
	{"max_messenger.go", "notifications"},
	{"mail.go", "notifications"},

	{"site_", "sitecontent"},
	{"game_", "games"},
	{"argument_clinic", "argumentclinic"},

	// Cross-cutting HTTP plumbing and operations.
	{"router.go", "kernel"},
	{"utils.go", "kernel"},
	{"types.go", "kernel"},
	{"middleware.go", "kernel"},
	{"request_observability.go", "kernel"},
	{"prometheus_metrics.go", "kernel"},
	{"runtime_metrics.go", "kernel"},
	{"health_deep.go", "kernel"},
	{"alert_window.go", "kernel"},
	{"debug_client_log.go", "kernel"},
}

// packageModule maps other internal packages (relative to apps/api) to modules.
var packageModule = map[string]string{
	"internal/aireport":      "chat", // dialog/lead summaries over chat data
	"internal/anonymizer":    "chat", // monthly irreversible anonymization of messages
	"internal/billing":       "billing",
	"internal/config":        "kernel",
	"internal/pseudonym":     "privacy",
	"internal/db":            "kernel",
	"internal/dbbackup":      "kernel",
	"internal/dbmaintenance": "kernel",
	"internal/temporaltest":  "kernel",
	"internal/testsupport":   "kernel",
	// Small helper subpackages that earlier refactors split out of httpapi.
	"internal/httpapi/admin":       "admin",
	"internal/httpapi/admin/users": "admin",
	"internal/httpapi/auth":        "auth",
	"internal/httpapi/chat":        "chat",
	"internal/httpapi/cms":         "sitecontent",
	"internal/httpapi/promocode":   "access",
	"internal/httpapi/public":      "modes",
	"internal/httpapi/webhook":     "billing",
	"cmd/api":                      "kernel",
	"cmd/worker":                   "kernel",
	".":                            "kernel",
}

// moduleForFile returns the module of a non-test Go file given its path
// relative to apps/api (slash-separated). ok=false means "not assigned".
func moduleForFile(rel string) (string, bool) {
	dir, file := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		dir = "."
	}
	switch {
	case strings.HasPrefix(dir, "internal/modules/"):
		return strings.SplitN(strings.TrimPrefix(dir, "internal/modules/"), "/", 2)[0], true
	case dir == "internal/kernel" || strings.HasPrefix(dir, "internal/kernel/"):
		return "kernel", true
	case dir == "internal/archtest":
		return "kernel", true
	case dir == "internal/httpapi":
		for _, r := range httpapiFileModule {
			if file == r.prefix || (strings.HasPrefix(file, r.prefix) && !strings.HasSuffix(r.prefix, ".go")) {
				return r.module, true
			}
		}
		return "", false
	}
	m, ok := packageModule[dir]
	return m, ok
}
