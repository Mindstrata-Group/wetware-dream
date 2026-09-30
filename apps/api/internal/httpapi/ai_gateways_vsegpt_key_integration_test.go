//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestVsegptKey_OnlyFromGatewayRegistry: the vsegpt key lives only in the
// gateway registry in the database.
//
// The scenario mirrors prod after rotation: the API starts without a key
// (main.go no longer reads OPENAI_API_KEY), the vsegpt registry row is empty:
// the gateway must count as unconfigured and drop out of the chain, not go to
// the network with an empty or old key. As soon as the admin writes a key into
// the registry row, the request goes out with exactly that key.
func TestVsegptKey_OnlyFromGatewayRegistry(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ctx := context.Background()

	var (
		mu       sync.Mutex
		gotAuth  []string
		requests int
	)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	t.Cleanup(fake.Close)

	// Handler built as in main.go: OpenAIAPIKey is not filled from anywhere.
	ts := NewTestServer(t, env.Pool)
	h := ts.Handler

	seedGateway(t, env.Pool, aiProviderVsegpt, gatewayProtocolOpenAI, 10)
	if _, err := env.Pool.Exec(ctx, `update ai_gateways set api_key = '', base_url = $1 where id = 'vsegpt'`, fake.URL); err != nil {
		t.Fatalf("подготовка пустого ключа: %v", err)
	}
	h.c.aiGateways.clear()

	if h.gatewayConfigured(ctx, aiProviderVsegpt) {
		t.Fatal("vsegpt без ключа в реестре считается настроенным")
	}
	for _, link := range h.buildProviderChain(ctx, aiCallSpec{Provider: aiProviderVsegpt, Model: "m"}) {
		if link.Provider == aiProviderVsegpt {
			t.Fatalf("vsegpt без ключа попал в цепочку: %+v", link)
		}
	}

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))
	code, body := httpJSON(t, ts, "GET", "/api/admin/ai-gateways", nil)
	if code != http.StatusOK {
		t.Fatalf("список шлюзов: %d body=%+v", code, body)
	}
	row := gatewayByIDInBody(t, body, aiProviderVsegpt)
	if row["apiKeySource"] != "none" || row["configured"] != false {
		t.Errorf("админка обязана показать vsegpt ненастроенным: source=%v configured=%v", row["apiKeySource"], row["configured"])
	}

	// The admin writes the new key into the registry row.
	if _, err := env.Pool.Exec(ctx, `update ai_gateways set api_key = 'db-rotated-key' where id = 'vsegpt'`); err != nil {
		t.Fatalf("запись ключа: %v", err)
	}
	h.c.aiGateways.clear()

	if !h.gatewayConfigured(ctx, aiProviderVsegpt) {
		t.Fatal("ключ в реестре не делает vsegpt настроенным")
	}
	content, _, _, _, err := h.doAIChatResilient(ctx, "m", 0.5, []map[string]string{{"role": "user", "content": "привет"}}, "")
	if err != nil {
		t.Fatalf("вызов через реестр: %v", err)
	}
	if content != "ok" {
		t.Errorf("content = %q", content)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests == 0 {
		t.Fatal("запрос до шлюза vsegpt не дошёл")
	}
	for _, a := range gotAuth {
		if a != "Bearer db-rotated-key" {
			t.Errorf("Authorization = %q, want ключ из реестра", a)
		}
	}
}
