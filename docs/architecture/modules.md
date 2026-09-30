# API modules: ownership and boundary debt

Source of truth for ownership: `apps/api/internal/archtest/ownership_map_test.go`.
Numbers below come from
`ARCHTEST_REPORT=1 go test ./internal/archtest/ -run TestReport -v`
(2026-09-30, staging `3ae46f1` + pilot). Re-run it after each module move and
update this page.

"Refs" = SQL string literals in non-test code. "Out" = this module querying
other modules' tables; "In" = other modules querying this module's tables.

## Module map

| Module | Owns tables | Code today (`internal/httpapi` unless noted) | Files / lines | Out | In |
|---|---|---|---|---|---|
| `kernel` | `system_settings`, `alembic_version` | `router.go`, `utils.go`, `types.go`, `middleware.go`, `request_observability.go`, `prometheus_metrics.go`, `runtime_metrics.go`, `health_deep.go`, `alert_window.go`, `debug_client_log.go`, `handler_caches.go`, `admin_cache_simple.go`, `admin_middleware_chain.go`; `internal/{config,db,dbbackup,dbmaintenance}`, `cmd/*`; new `internal/kernel/*` | 31 / 2 989 | 0 | 10 |
| `auth` | `users`, `auth_sessions`, `oauth_states`, `user_identities`, `password_reset_tokens`, `email_verification_tokens`, `jwt_token_usage`, `security_events`, `cookie_consents` | `auth_*.go`, `oauth_*.go`, `cookie_consent.go`, `bootstrap_handlers.go`, `httpapi/auth/` | 15 / 2 353 | 24 | 76 |
| `access` | `tariffs`, `tariff_groups`, `tariff_mode`, `user_mode_access`, `promocodes`, `promocode_targets`, `promocode_usages`, `daily_message_counts`, `daily_mode_usage`, `message_usage`, `dialog_message_access_usage`, `admin_mode_usage_resets` | `access*.go`, `promo_admin_handlers.go`, `public_tariffs.go`, `chat_quota_*.go`, `admin_promocode_*.go`, `admin_access_handlers.go`, `admin_tariff_handlers.go`, `admin_extra_tariff*.go`, `httpapi/promocode/` | 14 / 3 112 | 27 | 110 |
| `billing` | `invoices`, `payment_methods`, `subscriptions`, `autopay_subscriptions`, `billing_runs`, `billing_run_items`, `recurring_payment_attempts`, `yookassa_webhook_events`, `log_income` | `yookassa_*.go`, `billing_views.go`, `admin_payments_handlers.go`, `admin_access_recovery.go`, `httpapi/webhook/`, `internal/billing` | 12 / 3 445 | 25 | 6 |
| `chat` | `dialogs_messages`, `users_dialogs`, `chat_files`, `chat_file_blobs`, `chat_message_attachments`, `orchestration_decision_logs`, `message_terminologies`, `group_chats` | `chat_*.go` (minus quota), `handlers.go` (grab-bag, to split), `httpapi/chat/`, `internal/aireport`, `internal/anonymizer` | 15 / 3 315 | 25 | 29 |
| `modes` | `modes`, `prompts`, `public_demo_modes_cache`, `admin_summary_prompts`, `mode_reminder_logs`, `mode_usage_reminders` | `chat_modes.go`, `mode_history.go`, `public.go`, `admin_mode_*.go`, `httpapi/public/` | 6 / 1 367 | 14 | 35 |
| `aigateway` | `ai_gateways`, `ai_provider_events` | `ai_*.go`, `admin_ai_*.go` | 14 / 3 006 | 3 | 1 |
| `notifications` | `notification_*` (8), `push_subscriptions`, `reminder_configs`, `reminder_logs`, `reminder_message_logs`, `user_inactivity_reminders`, `admin_notification_sends` | `notifications.go`, `notification_queue.go`, `lead_notifications.go`, `telegram*.go`, `max_messenger.go`, `mail.go`, `admin_broadcast_summary.go` | 8 / 2 878 | 49 | 0 |
| `sitecontent` | `site_content`, `site_media` | `site_content.go`, `site_media.go`, `httpapi/cms/` | 3 / 476 | 0 | 2 |
| `games` | `game_results`, `game_sessions`, `game_tasks` | `game_*.go` | 5 / 2 142 | 1 | 1 |
| `admin` | `admin_audit_log`, `admin_export_summaries`, `tunnel_machines` | remaining `admin_*.go`, `mcp_*.go`, `tester_*.go`, `expert_handlers.go`, `httpapi/admin/` | 20 / 3 390 | 107 | 5 |
| `argumentclinic` | `argument_clinic_votes` | **moved:** `internal/modules/argumentclinic` | 1 / 159 | 0 | 0 |

Totals: 71 tables, 12 modules, **71 cross-module pairs, 275 SQL literals**.

Notes:

- `handlers.go` holds the `Handler` struct *and* chat entry points
  (`StartChat`, `SelectMode`); it is counted under `chat` and must be split
  when chat moves.
- `internal/anonymizer` and `internal/aireport` are counted under `chat`
  because they read messages; the Python anonymizer service will own its own
  vault table when it lands.
- `system_settings` (10 refs from access, admin, aigateway, billing, chat) is
  an installation-wide key/value store. It needs a small kernel settings API,
  not a module.

## Boundary debt: top 10 pairs

| # | Module → table (owner) | Refs | Files | What replaces it |
|---|---|---|---|---|
| 1 | notifications → `users` (auth) | 27 | 6 | `auth.Contacts(userIDs)` / `auth.User(id)` read API |
| 2 | admin → `users` (auth) | 19 | 8 | same auth read API + paging query for lists |
| 3 | chat → `users` (auth) | 12 | 4 | `auth.User(id)`; guest creation via `auth.EnsureGuest` |
| 4 | admin → `modes` (modes) | 11 | 8 | `modes` admin query API |
| 5 | access → `modes` (modes) | 10 | 7 | `modes.Exists/Active(ids)` |
| 6 | admin → `promocodes` (access) | 10 | 6 | move the promo admin screens into `access` |
| 7 | admin → `user_mode_access` (access) | 9 | 5 | `access.Grants(userID)` |
| 8 | admin → `tariffs` (access) | 8 | 7 | `access` tariff query API |
| 9 | admin → `dialogs_messages` (chat) | 7 | 6 | `chat` read API for dialog views/exports |
| 9 | admin → `message_usage` (access) | 7 | 2 | `access.Usage(userID, period)` |
| 9 | admin → `tariff_mode` (access) | 7 | 4 | `access` tariff query API |
| 9 | admin → `users_dialogs` (chat) | 7 | 6 | `chat` read API |
| 9 | billing → `tariffs` (access) | 7 | 4 | `access.Tariff(id)` |
| 9 | billing → `user_mode_access` (access) | 7 | 3 | `access.GrantPaidAccess(...)` in the payment transaction |

By owner: access 110, auth 76, modes 35, chat 29, kernel 10, billing 6,
admin 5, sitecontent 2, aigateway 1, games 1.
By reader: admin 107, notifications 49, access 27, billing 25, chat 25,
auth 24, modes 14, aigateway 3, games 1.

Full list: `apps/api/internal/archtest/testdata/cross_module_table_access.txt`.

## Transactions that cross modules

Some writes must stay atomic across two modules (payment succeeded → access
granted; promo applied → usage counted and access granted). The rule for
those: the owning module exposes a function that accepts the caller's
`pgx.Tx`, e.g. `access.GrantPaidAccessTx(ctx, tx, ...)`. One database, one
transaction, but only the owner writes its tables.

## Move order

See [modular-monolith-rollout.md](modular-monolith-rollout.md).
