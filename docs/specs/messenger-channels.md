# Messenger channels in the admin panel

> ## ⚠️ Status: specification, not implemented
>
> This document fixes the agreed behaviour. There is no feature code yet.
> The acceptance criteria below already have stub tests that skip with
> `SPEC messenger-channels AC-N: not implemented` (see "Stub tests" at the
> end). Implementation starts by removing `t.Skip` from a test and writing
> code until it passes.

Maintained since 2026-09-30.

## 1. What changes for the admin

Today Telegram and Max bots are configured only through server environment
variables (`TELEGRAM_BOT_TOKEN`, `MAX_BOT_TOKEN` and friends). Replacing a
token, adding a second bot, or sending bot traffic through your own proxy
needs someone with server access and an API restart.

With this feature the admin does it in the admin panel, "System →
Messengers":

- connect a bot: type, title, token;
- if the messenger is unreachable from the server's network, set your own
  proxy or relay (for example, a VPS at a hosting provider) and tick
  "Through proxy";
- press "Check" and see whether the bot answers;
- disable, archive and restore a channel without touching the server.

The model is the AI gateway registry (`ai_gateways`, "Orchestration → AI"):
the secret lives only in the database, leaves it only masked, an empty form
field does not overwrite it, and the second address plus its checkbox switch
without losing the setting.

## 2. What exists in the code today (2026-09-30)

| What | Where | Configured by |
|---|---|---|
| Telegram: forwarding user messages to an admin chat | `chat_history.go` `forwardUserMessageToTelegram` | `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` |
| Telegram: linking a user account via `getUpdates` | `telegram_link.go` | `TELEGRAM_BOT_TOKEN`, `TELEGRAM_BOT_USERNAME` |
| Telegram: Bot API base URL (a proxy in production when api.telegram.org is blocked) | `telegram.go` `telegramAPIBase` | `TELEGRAM_API_BASE_URL` |
| Max: linking, webhook, notifications | `max_messenger.go`, `router.go` | `MAX_BOT_TOKEN`, `MAX_BOT_USERNAME` |
| Per-channel notification queue | `notification_queue.go` (`notification_channel_queue.channel`) | — |
| Lead notifications per mode | `lead_notifications.go` (`modes.lead_notify_*`) | chats in the mode, tokens in env |

All tokens are read from env at startup. `system_settings` holds no
messenger tokens.

Defects found along the way (the spec closes them; they can be fixed
earlier):

1. **The Max webhook path is the first 20 characters of the bot token**
   (`router.go`, `MaxWebhook`). The token prefix ends up in Caddy access logs
   and in any proxy on the way, and changing the token silently changes the
   webhook address.
2. **The Telegram token is part of the URL path** (`/bot<token>/sendMessage`).
   An `http.Client.Do` error is a `*url.Error` with the full URL, and
   `lead_notifications.go` logs it with `%v`. A network error puts the token
   into the log.
3. The HMAC key for Max link tokens is the bot token itself
   (`maxUserToken`). Changing the token invalidates pending link URLs —
   acceptable, but it must be a conscious choice.

## 3. Model: messenger channel

Table `messenger_channels`:

| Column | Type | Meaning |
|---|---|---|
| `id` | text pk, `^[a-z0-9][a-z0-9_-]{1,31}$` | same as gateways |
| `kind` | text, `telegram` \| `max` | API format; a new messenger is a new adapter in code, not a database row |
| `title` | text | name shown to the admin |
| `bot_token` | text, default `''` | **database only**; exposed as mask `****1234` |
| `bot_username` | text | for `t.me/…` and `max.ru/…` links |
| `admin_chat_id` | text | where admin forwarding goes (replaces `TELEGRAM_CHAT_ID`) |
| `api_base_url` | text | direct API address; empty means the default for `kind` |
| `relay_url` | text, default `''` | relay address that replaces the API base (as `TELEGRAM_API_BASE_URL` does today) |
| `proxy_url` | text, default `''` | outbound proxy `http`/`https`/`socks5`/`socks5h`, e.g. a VPS at a hosting provider; may contain credentials, so it is a secret too |
| `route` | text, `direct` \| `relay` \| `proxy`, default `direct` | which path to use; addresses are **kept** when switching |
| `webhook_secret` | text | random webhook path secret, ≥ 32 bytes, base64url; **not derived from the token** |
| `priority` | int | list order; a new channel goes last |
| `enabled` | bool | |
| `archived_at` | timestamptz | archive instead of delete: user links and the queue refer to the channel |
| `created_at`, `updated_at` | timestamptz | |

Constraint: **at most one enabled, non-archived channel per `kind`**
(partial unique index). Reason: `users.telegram_id` and `users.max_chat_id`
are bound to a specific bot. Two active bots of the same messenger would mean
half of the users get notifications from a bot they never started, and
Telegram will not deliver those. A second channel of the same kind is created
disabled, as a spare. Switching to it is an explicit action with a warning:
"Users will have to reconnect notifications".

Address validation (`relay_url`, `proxy_url`, `api_base_url`):

- `relay_url` and `api_base_url` — `https` only;
- `proxy_url` — `http`, `https`, `socks5`, `socks5h`;
- always rejected: loopback, link-local (including `169.254.169.254`),
  `0.0.0.0`/`::`, multicast;
- private networks (RFC 1918, ULA) are rejected except hosts listed in the
  infrastructure setting `MESSENGER_PRIVATE_HOSTS_ALLOW`. This allows, for
  example, a proxy on the docker bridge, while the list itself stays a
  server setting, not an admin-panel one;
- the check is repeated **at dial time** against the actual IP, otherwise a
  DNS name that pointed outside on save can later be moved to an internal
  address.

## 4. Admin API

Every endpoint requires the `system` admin section:
`requireAdminSection(w, r, "system", write)`. No session → `401`, no section
→ `403`. Bodies and errors have the same shape as `/api/admin/ai-gateways`.

| Method and path | What it does | Responses |
|---|---|---|
| `GET /api/admin/messenger-channels` | list including archived; fields `tokenMasked`, `tokenSource` (`channel` \| `env` \| `none`), `effectiveAddress`, `route`, `webhookUrl` for Max | 200 |
| `POST /api/admin/messenger-channels` | create; goes last, disabled if a channel of this `kind` is already enabled | 200, 400 (id/kind/address), 409 (id taken) |
| `POST /api/admin/messenger-channels/{id}` `action=update` | edit fields; `botToken: ""` or a missing field keeps the stored token; `clearToken: true` clears it explicitly | 200, 400, 404, 409 (second enabled of the same `kind`) |
| `… action=archive` / `restore` | archive and restore; an archived channel neither sends nor accepts webhooks | 200, 404 |
| `… action=move` `direction=up\|down` | order; no-op at the edge | 200, 404 |
| `… action=rotate-webhook-secret` | new secret; the old path answers 404 immediately | 200, 404 |
| `… action=check` | `getMe` (Telegram) or `GET /me` (Max) over the selected route, 10 s timeout; answers `{ok, botUsername}` or a human-readable error without the token | 200 (check result inside), 404 |

Every change writes `writeAdminAudit` (`admin.messenger_channel.*`)
**without** the token, the proxy password and the webhook secret.

The channel cache is handler-local, like `aiGatewaysCache`. An admin write
clears it, and the next send sees the new token without a restart.

The Max webhook is routed dynamically: `/webhooks/max/{secret}` looks the
channel up by `webhook_secret` in the cache. Registering the path from env at
startup goes away.

## 5. Admin screen

"System → Messengers". Short copy about what changes for people, without
words like "webhook", "relay", "endpoint":

- channel list: title, type, "On"/"Off", "Bot answers"/"Bot does not
  answer" (last check), where requests go;
- form: title, token (empty field, saved mask as placeholder), bot name,
  forwarding chat, "Through proxy" + address;
- buttons "Check", "Archive", "Restore", order arrows;
- enabling a second bot of the same messenger asks to confirm "Users will
  have to reconnect notifications".

**No empty placeholder screen before implementation.** The section appears
together with a working API.

## 6. Migration and moving tokens

1. SQL file in `apps/api/sql/` (idempotent, `create table if not exists`),
   `internal/testsupport/schema_base.sql` in sync, `migration_safety` checks.
   **Not applied automatically on stage or prod** (AGENTS.md): an operator
   applies it manually, the drift check catches divergence.
2. Seeds: one `telegram` row and one `max` row with an **empty** token. SQL
   cannot see env, and copying a secret for the sake of a migration is
   pointless.
3. **Phase 1, transitional.** If the row token is empty, the code reads the
   old env variable (`TELEGRAM_BOT_TOKEN`, etc.) and the admin panel shows
   `tokenSource=env` — "Token from server settings". Production keeps working
   as before.
4. The admin pastes tokens into the admin panel. For Max this means a new
   webhook: after saving, the code registers the address with
   `webhook_secret` at Max.
5. **Phase 2.** Reading env for messenger tokens is removed (as done for
   vsegpt), the variables leave `docker-compose.prod.yml` and the server
   `.env`. `tokenSource=env` no longer occurs.

Rolling back phase 1: disable the channel in the admin panel; with an empty
row token the code falls back to env.

## 7. Risks and how they are closed

| Risk | Mitigation | AC |
|---|---|---|
| Token in an API response | mask only; `tokenSource` instead of the value | AC-3 |
| Token in logs via `*url.Error` | a single send-error wrapper strips `/bot<token>` and the `Authorization` header; the test captures the log | AC-14 |
| Token prefix in the webhook path | random `webhook_secret`, rotation button | AC-12 |
| SSRF via proxy or relay address | scheme allow-list, internal addresses rejected on save **and** at dial time | AC-6, AC-7 |
| Two bots of one messenger silently split users | partial unique index + confirmation in UI | AC-8, UI-2 |
| An empty form field wiped the token | empty = "keep"; clearing only with an explicit flag | AC-4 |
| Turning the proxy off wiped its address | `route` separate from addresses | AC-5 |
| The migration silently hit production | manual apply, drift check | AC-17 |

## 8. Acceptance criteria

Each criterion has a test with the same number. API level — Go: unit in
`messenger_channels_spec_test.go`, integration in
`messenger_channels_spec_integration_test.go`. Only what can be checked
exclusively in the UI — vitest.

| AC | Criterion | Test |
|---|---|---|
| AC-1 | List and writes require the `system` section: no session 401, no section 403, admin 200 | integration |
| AC-2 | Create: unknown `kind` or bad id → 400, taken id → 409, new channel last, audit without token | integration |
| AC-3 | The token never leaves in clear: only `tokenMasked` and `tokenSource` in list, create, update and check | integration |
| AC-4 | Empty or missing `botToken` on update keeps the stored one; `clearToken:true` clears it | integration |
| AC-5 | Switching `route` keeps `relay_url`/`proxy_url`; `effectiveAddress` shows the actual path | integration |
| AC-6 | Address check on save: schemes, loopback, link-local, metadata, unspecified, multicast → 400; private networks only from `MESSENGER_PRIVATE_HOSTS_ALLOW` | unit |
| AC-7 | Dial-time check: a name resolving to an internal address is refused at dial | unit |
| AC-8 | A second enabled channel of the same `kind` → 409 with a clear error | integration |
| AC-9 | An archived channel neither sends nor accepts webhooks; `restore` brings it back | integration |
| AC-10 | Sending uses the database token over the selected route; env is ignored when the row has a token | integration |
| AC-11 | Phase 1: empty row token → env, `tokenSource=env`. Phase 2: env is not read at all | integration |
| AC-12 | `webhook_secret` is random, ≥ 32 bytes, not from the token; rotation → old path 404; old token-prefix path → 404 | integration |
| AC-13 | The Max webhook is looked up by the secret from the database; changing the secret works without a restart | integration |
| AC-14 | Send errors and logs contain neither the token nor the proxy password | unit |
| AC-15 | `check` uses the selected route, finishes within 10 s, answers `botUsername` or a human-readable error without secrets | integration |
| AC-16 | An admin write clears the handler-local cache: the next send uses the new token | integration |
| AC-17 | Migration is idempotent, `schema_base.sql` in sync, `migration_safety` green, seeds with an empty token | integration |
| UI-1 | Token field is empty with the mask as placeholder; saving without typing does not send `botToken` | vitest |
| UI-2 | Enabling a second bot of the same messenger requires confirmation mentioning reconnecting notifications | vitest |

## 9. Stub tests

- `apps/api/internal/httpapi/messenger_channels_spec_test.go` — AC-6, AC-7, AC-14;
- `apps/api/internal/httpapi/messenger_channels_spec_integration_test.go` — the other AC;
- `apps/web/src/app/admin/components/AdminMessengerChannelsSection.test.tsx` — UI-1, UI-2 (`it.todo`).

Find them all: `grep -rn "SPEC messenger-channels" apps/`. CI lists the
skipped SPEC tests in a dedicated step of the `api-unit` job.
