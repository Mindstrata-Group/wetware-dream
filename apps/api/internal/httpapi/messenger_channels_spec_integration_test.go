//go:build integration

package httpapi

import "testing"

// Acceptance-criteria stubs from docs/specs/messenger-channels.md that need a
// database and an HTTP server. The feature is not implemented — every test
// skips BEFORE touching the database, so the stubs are visible even in a run
// without TEST_DATABASE_URL.
//
// When implementing, follow admin_ai_gateways_integration_test.go:
// testsupport.NewEnv(t) for an isolated database, NewFactory/NewTestServer,
// a dedicated newHandlerCaches(), and a fake Bot API on httptest.Server
// instead of api.telegram.org.

// AC-1: no session → 401, user without the "system" admin section → 403,
// admin → 200; for the GET list and for every write
// (create/update/archive/move/check).
func TestSpecMessengerChannels_AC01_RequiresSystemSection(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-1")
}

// AC-2: kind outside telegram/max or an id not matching the pattern → 400;
// an id already taken (including archived) → 409; a new channel is last in
// the list; the admin.messenger_channel.create audit record exists and holds
// no token.
func TestSpecMessengerChannels_AC02_CreateValidationOrderAndAudit(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-2")
}

// AC-3: the token value appears in no response — list, create, update,
// check; tokenMasked looks like ****1234 and tokenSource is present.
func TestSpecMessengerChannels_AC03_TokenOnlyMasked(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-3")
}

// AC-4: an update without botToken or with botToken "" keeps the stored
// token; clearToken:true clears it and tokenSource becomes env/none.
func TestSpecMessengerChannels_AC04_EmptyTokenFieldKeepsStored(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-4")
}

// AC-5: route proxy → direct → proxy keeps proxy_url and relay_url;
// effectiveAddress in the list matches where the request actually went.
func TestSpecMessengerChannels_AC05_RouteSwitchKeepsAddresses(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-5")
}

// AC-8: with a telegram channel enabled, enabling a second telegram channel
// (create enabled or update enabled=true) → 409 with a human-readable error;
// a channel of another kind is enabled without restriction.
func TestSpecMessengerChannels_AC08_OneEnabledChannelPerKind(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-8")
}

// AC-9: an archived channel sends nothing (the fake Bot API gets no
// requests) and its webhook answers 404; restore brings both back.
func TestSpecMessengerChannels_AC09_ArchivedChannelIsSilent(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-9")
}

// AC-10: with a token in the row and a different token in env, a
// notification goes out with the database token over the selected route
// (direct / relay / proxy — three fake servers, exactly one gets the call).
func TestSpecMessengerChannels_AC10_SendUsesChannelTokenAndRoute(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-10")
}

// AC-11: phase 1 — empty token in the row, token in env → sending uses the
// env token and the list shows tokenSource=env. Phase 2 (after production
// has moved) — same scenario: nothing is sent, tokenSource=none.
func TestSpecMessengerChannels_AC11_EnvFallbackThenRemoval(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-11")
}

// AC-12: webhook_secret is ≥ 32 bytes base64url and contains no token
// prefix; rotate-webhook-secret → old path 404, new path 200;
// /webhooks/max/<first 20 token chars> → 404.
func TestSpecMessengerChannels_AC12_RandomWebhookSecretAndRotation(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-12")
}

// AC-13: the Max webhook resolved by the secret from the database works on
// an already running server: changing the secret or token in the admin
// panel needs no new router.
func TestSpecMessengerChannels_AC13_WebhookResolvedFromDatabase(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-13")
}

// AC-15: check uses the selected route, answers with an error within 10 s
// when the Bot API hangs, and with ok + botUsername when it is alive; no
// secrets in the response.
func TestSpecMessengerChannels_AC15_CheckActionUsesRouteWithTimeout(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-15")
}

// AC-16: after a token update in the admin panel, the next send by the same
// Handler uses the new token — the write clears the handler-local cache.
func TestSpecMessengerChannels_AC16_AdminWriteInvalidatesCache(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-16")
}

// AC-17: the SQL migration is idempotent (applying twice is not an error),
// schema_base.sql contains messenger_channels, telegram and max are seeded
// with an empty token, and the partial unique index on kind exists.
func TestSpecMessengerChannels_AC17_MigrationIdempotentAndSeeded(t *testing.T) {
	t.Parallel()
	specMessengerChannelsSkip(t, "AC-17")
}
