//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"mindstrata-stage1/api/internal/testsupport"
)

// gatewayIDs pulls the gateway order out of the list response: tests about
// order should read as a statement about order, not as JSON parsing.
func gatewayIDs(t *testing.T, body map[string]any) []string {
	t.Helper()
	raw, ok := body["gateways"].([]any)
	if !ok {
		t.Fatalf("в ответе нет списка шлюзов: %+v", body)
	}
	out := []string{}
	for _, item := range raw {
		g, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("элемент списка не объект: %+v", item)
		}
		out = append(out, g["id"].(string))
	}
	return out
}

func gatewayByIDInBody(t *testing.T, body map[string]any, id string) map[string]any {
	t.Helper()
	raw, _ := body["gateways"].([]any)
	for _, item := range raw {
		g := item.(map[string]any)
		if g["id"] == id {
			return g
		}
	}
	t.Fatalf("шлюз %q не найден в ответе: %+v", id, body)
	return nil
}

// TestAdminAIGateways_CreateAppearsLastAndCanBeRaised: the main scenario the
// whole thing was built for: add a gateway by hand and raise it in the list.
//
// A new gateway must go to the END: it is added as a spare, and silently moving
// all traffic onto an unverified gateway would be the worst default.
func TestAdminAIGateways_CreateAppearsLastAndCanBeRaised(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	seedGateway(t, env.Pool, "anthropic", "anthropic", 10)
	seedGateway(t, env.Pool, "vsegpt", "openai", 30)

	code, body := httpJSON(t, ts, "POST", "/api/admin/ai-gateways", map[string]any{
		"id":           "relay-fi",
		"title":        "Зарубежный релей",
		"protocol":     "openai",
		"baseUrl":      "https://relay.example/v1",
		"apiKey":       "relay-secret",
		"defaultModel": "openai/gpt-5",
	})
	if code != http.StatusOK {
		t.Fatalf("создание шлюза: %d body=%+v", code, body)
	}

	code, body = httpJSON(t, ts, "GET", "/api/admin/ai-gateways", nil)
	if code != http.StatusOK {
		t.Fatalf("список шлюзов: %d body=%+v", code, body)
	}
	if got, want := gatewayIDs(t, body), []string{"anthropic", "vsegpt", "relay-fi"}; !equalStrings(got, want) {
		t.Fatalf("порядок после создания = %v, want %v (новый шлюз идёт последним)", got, want)
	}

	// The key is only ever returned masked.
	created := gatewayByIDInBody(t, body, "relay-fi")
	if created["apiKeyMasked"] == "relay-secret" {
		t.Error("ключ ушёл в ответ админки в открытом виде")
	}
	if created["apiKeySource"] != "gateway" {
		t.Errorf("apiKeySource = %v, want gateway (ключ задан в самой строке)", created["apiKeySource"])
	}

	// Raise it twice: first above vsegpt, then above anthropic.
	for i := 0; i < 2; i++ {
		code, body = httpJSON(t, ts, "POST", "/api/admin/ai-gateways/relay-fi", map[string]any{
			"action": "move", "direction": "up",
		})
		if code != http.StatusOK {
			t.Fatalf("подъём #%d: %d body=%+v", i+1, code, body)
		}
	}
	_, body = httpJSON(t, ts, "GET", "/api/admin/ai-gateways", nil)
	if got, want := gatewayIDs(t, body), []string{"relay-fi", "anthropic", "vsegpt"}; !equalStrings(got, want) {
		t.Fatalf("порядок после двух подъёмов = %v, want %v", got, want)
	}

	// The edge of the list is not an error: nowhere to move, but the request is valid.
	code, body = httpJSON(t, ts, "POST", "/api/admin/ai-gateways/relay-fi", map[string]any{
		"action": "move", "direction": "up",
	})
	if code != http.StatusOK {
		t.Fatalf("подъём с верхней позиции должен быть no-op, а не ошибкой: %d body=%+v", code, body)
	}
	_, body = httpJSON(t, ts, "GET", "/api/admin/ai-gateways", nil)
	if got, want := gatewayIDs(t, body), []string{"relay-fi", "anthropic", "vsegpt"}; !equalStrings(got, want) {
		t.Fatalf("порядок изменился на краю списка: %v, want %v", got, want)
	}

	// The registry really reached the fallback chain.
	h := Handler{DB: env.Pool, c: newHandlerCaches()}
	chain := h.buildProviderChain(ctx, aiCallSpec{Provider: "anthropic", Model: "claude-sonnet-5"})
	if len(chain) == 0 {
		t.Fatal("цепочка пуста: реестр не доехал до сборки звеньев")
	}
	foundRelay := false
	for _, link := range chain {
		if link.Provider == "relay-fi" {
			foundRelay = true
			if link.Protocol != gatewayProtocolOpenAI {
				t.Errorf("протокол звена relay-fi = %q, want openai", link.Protocol)
			}
		}
	}
	if !foundRelay {
		t.Errorf("заведённый шлюз не попал в цепочку: %+v", chain)
	}
}

// TestAdminAIGateways_ArchiveHidesFromChainAndRestoreBringsBack: archive instead
// of delete: the gateway leaves the chain, but the row stays and comes back.
func TestAdminAIGateways_ArchiveHidesFromChainAndRestoreBringsBack(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	seedGateway(t, env.Pool, "anthropic", "anthropic", 10)
	seedGateway(t, env.Pool, "old-relay", "openai", 20)

	code, body := httpJSON(t, ts, "POST", "/api/admin/ai-gateways/old-relay", map[string]any{"action": "archive"})
	if code != http.StatusOK {
		t.Fatalf("архивация: %d body=%+v", code, body)
	}

	h := Handler{DB: env.Pool, c: newHandlerCaches()}
	for _, g := range h.activeGateways(ctx) {
		if g.ID == "old-relay" {
			t.Fatal("архивный шлюз остался в рабочем реестре")
		}
	}

	// The row is not deleted: the admin still sees it, marked.
	_, body = httpJSON(t, ts, "GET", "/api/admin/ai-gateways", nil)
	archived := gatewayByIDInBody(t, body, "old-relay")
	if archived["archived"] != true {
		t.Errorf("архивный шлюз не помечен в списке: %+v", archived)
	}

	code, body = httpJSON(t, ts, "POST", "/api/admin/ai-gateways/old-relay", map[string]any{"action": "restore"})
	if code != http.StatusOK {
		t.Fatalf("восстановление: %d body=%+v", code, body)
	}
	restored := Handler{DB: env.Pool, c: newHandlerCaches()}
	found := false
	for _, g := range restored.activeGateways(ctx) {
		if g.ID == "old-relay" {
			found = true
		}
	}
	if !found {
		t.Error("восстановленный шлюз не вернулся в рабочий реестр")
	}
}

// TestAdminAIGateways_LastAliveGatewayCannotBeArchived: archiving the last
// working gateway means stopping the chat, and the user would notice it, not
// the admin. So the request is rejected.
func TestAdminAIGateways_LastAliveGatewayCannotBeArchived(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	seedGateway(t, env.Pool, "only-one", "openai", 10)

	code, body := httpJSON(t, ts, "POST", "/api/admin/ai-gateways/only-one", map[string]any{"action": "archive"})
	if code != http.StatusBadRequest {
		t.Fatalf("архивация последнего шлюза: %d body=%+v, want 400", code, body)
	}
}

// TestAdminAIGateways_RejectsUnknownProtocolAndBadID: the protocol is code, not
// a free-form field. The refusal must be clear, not a 500 from the driver.
func TestAdminAIGateways_RejectsUnknownProtocolAndBadID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	code, body := httpJSON(t, ts, "POST", "/api/admin/ai-gateways", map[string]any{
		"id": "custom-llm", "title": "Свой", "protocol": "mistral-native",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("неизвестный протокол: %d body=%+v, want 400", code, body)
	}

	code, body = httpJSON(t, ts, "POST", "/api/admin/ai-gateways", map[string]any{
		"id": "Шлюз Мой", "title": "Свой", "protocol": "openai",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("некорректный id: %d body=%+v, want 400", code, body)
	}
}

// TestAdminAIGateways_EmptyAPIKeyDoesNotWipeStoredOne: an empty form field means
// 'untouched, there is a mask', not 'erase the key'. Otherwise saving any
// neighbouring field would silently cut the gateway off.
func TestAdminAIGateways_EmptyAPIKeyDoesNotWipeStoredOne(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	seedGateway(t, env.Pool, "relay-fi", "openai", 10)
	if _, err := env.Pool.Exec(ctx, `update ai_gateways set api_key = 'stored-key' where id = 'relay-fi'`); err != nil {
		t.Fatalf("подготовка ключа: %v", err)
	}

	code, body := httpJSON(t, ts, "POST", "/api/admin/ai-gateways/relay-fi", map[string]any{
		"action": "update", "title": "Переименованный", "apiKey": "",
	})
	if code != http.StatusOK {
		t.Fatalf("правка шлюза: %d body=%+v", code, body)
	}

	var key, title string
	if err := env.Pool.QueryRow(ctx, `select api_key, title from ai_gateways where id = 'relay-fi'`).Scan(&key, &title); err != nil {
		t.Fatalf("чтение шлюза: %v", err)
	}
	if key != "stored-key" {
		t.Errorf("ключ затёрт пустым полем формы: %q", key)
	}
	if title != "Переименованный" {
		t.Errorf("название не сохранилось: %q", title)
	}
}

// seedGateway inserts a registry row directly: tests need a predictable
// starting set, not migration seeds (schema_base.sql deliberately has none).
func seedGateway(t *testing.T, pool *pgxpool.Pool, id, protocol string, priority int) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		insert into ai_gateways (id, title, protocol, api_key, priority, enabled)
		values ($1, $1, $2, 'test-key', $3, true)
		on conflict (id) do update set protocol = excluded.protocol, priority = excluded.priority`,
		id, protocol, priority)
	if err != nil {
		t.Fatalf("seedGateway(%q): %v", id, err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
