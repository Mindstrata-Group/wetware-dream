//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// SLAB D — ai_runtime + chat_orchestration
// =============================================================================

// vsegptRT: rewrites api.vsegpt.ru + api.openai.com → fake server.
type vsegptRT struct{ target string }

func (r vsegptRT) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if strings.Contains(host, "vsegpt.ru") || strings.Contains(host, "openai.com") {
		u := *req.URL
		u.Scheme = "http"
		u.Host = strings.TrimPrefix(r.target, "http://")
		req.URL = &u
		req.Host = u.Host
	}
	return http.DefaultTransport.RoundTrip(req)
}

// fakeVseGPT returns httptest server emulating OpenAI-compatible /v1/chat/completions.
func fakeVseGPT(t *testing.T, status int, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status >= 200 && status < 300 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": content}},
				},
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 5,
					"total_tokens":      15,
					"cost":              0.001,
				},
			})
		} else {
			_, _ = w.Write([]byte(`{"error":"upstream error"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 1. echoTestCompletion: returns the last user message.
func TestEchoTestCompletion(t *testing.T) {
	t.Parallel()
	h := Handler{}
	out := h.echoTestCompletion("any", []map[string]string{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "..."},
		{"role": "user", "content": "FINAL"},
	})
	if !strings.Contains(out, "FINAL") {
		t.Errorf("expected last user content 'FINAL', got %q", out)
	}
	if !strings.Contains(out, "[ECHO TEST]") {
		t.Errorf("expected ECHO TEST prefix, got %q", out)
	}

	// no user → "ok"
	out2 := h.echoTestCompletion("m", []map[string]string{{"role": "system", "content": "s"}})
	if !strings.Contains(out2, "ok") {
		t.Errorf("no-user fallback: %q", out2)
	}
}

// 2. doAIChat: empty API key → error (the only gate, 2026-05-29).
func TestDoAIChat_NoAPIKey(t *testing.T) {
	t.Parallel()
	h := Handler{OpenAIAPIKey: ""}
	_, _, _, _, err := h.doAIChat(context.Background(), "m", 0.5, nil, "x")
	if err == nil {
		t.Errorf("expected error with no api key")
	}
	if !strings.Contains(err.Error(), "vsegpt API key missing") {
		t.Errorf("error message should mention missing vsegpt key: got %v", err)
	}
}

// 4. doOpenAIChat: happy path through mock VseGPT.
func TestDoOpenAIChat_HappyPath(t *testing.T) {
	t.Parallel()
	fake := fakeVseGPT(t, 200, "fake-ai-response")
	h := Handler{
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	content, pt, ct, cost, err := h.doOpenAIChat(context.Background(),
		"gpt-4o-mini", 0.5,
		[]map[string]string{{"role": "user", "content": "hi"}}, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if content != "fake-ai-response" {
		t.Errorf("content: got %q", content)
	}
	if pt != 10 || ct != 5 {
		t.Errorf("tokens: got pt=%d ct=%d want 10,5", pt, ct)
	}
	if cost != 0.001 {
		t.Errorf("cost: got %v", cost)
	}
}

// 5. doOpenAIChat: 4xx → liveAIError with debug.
func TestDoOpenAIChat_4xxError(t *testing.T) {
	t.Parallel()
	fake := fakeVseGPT(t, 401, "")
	h := Handler{
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	_, _, _, _, err := h.doOpenAIChat(context.Background(),
		"gpt-4o-mini", 0.5,
		[]map[string]string{{"role": "user", "content": "hi"}}, "test-title")
	if err == nil {
		t.Fatalf("expected error")
	}
	var liveErr *liveAIError
	if !errors.As(err, &liveErr) {
		t.Fatalf("expected liveAIError, got %T", err)
	}
	if liveErr.Debug.Status != 401 {
		t.Errorf("debug status: %d want 401", liveErr.Debug.Status)
	}
	if liveErr.Debug.XTitle != "test-title" {
		t.Errorf("debug xTitle: %q", liveErr.Debug.XTitle)
	}
}

// 6. callLiveAI: integration through mock VseGPT.
func TestCallLiveAI_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "RuntimeTestMode"})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_ = f.AppendMessage(dialog.ID, "user", "previous q")
	_ = f.AppendMessage(dialog.ID, "assistant", "previous a")

	fake := fakeVseGPT(t, 200, "live-response")
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	modeData := modeRow{ID: mode.ID, Name: mode.Name, Prompt: "test prompt", AIModel: "gpt-4o-mini", Temperature: 0.5}
	answer, _, _, _, err := h.callLiveAI(context.Background(), user.ID, dialog.ID, modeData, "current q")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if answer != "live-response" {
		t.Errorf("got %q want live-response", answer)
	}
}

// 7. chatLiveAIDebug: admin → debug visible.
func TestChatLiveAIDebug_AdminSeesDebug(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	liveErr := &liveAIError{Message: "oops", Debug: liveAIDebugInfo{Provider: "p", URL: "/u"}}
	got, ok := h.chatLiveAIDebug(context.Background(), admin.ID, liveErr)
	if !ok {
		t.Fatalf("admin should see debug")
	}
	if got.Provider != "p" {
		t.Errorf("debug not propagated: %+v", got)
	}
}

// 8. chatLiveAIDebug: regular user → no debug.
func TestChatLiveAIDebug_UserNoDebug(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{Role: "user"})
	liveErr := &liveAIError{Message: "oops", Debug: liveAIDebugInfo{Provider: "p"}}
	_, ok := h.chatLiveAIDebug(context.Background(), user.ID, liveErr)
	if ok {
		t.Errorf("regular user got debug")
	}
}

// 9. chatLiveAIDebug: not a liveAIError → false.
func TestChatLiveAIDebug_PlainError(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	_, ok := h.chatLiveAIDebug(context.Background(), admin.ID, errors.New("plain"))
	if ok {
		t.Errorf("plain error should not produce debug")
	}
}

// 10. liveAIErrorResponse: extracts debug.
func TestLiveAIErrorResponse(t *testing.T) {
	t.Parallel()
	liveErr := &liveAIError{Message: "x", Debug: liveAIDebugInfo{Provider: "openai-compatible"}}
	resp := liveAIErrorResponse(liveErr)
	if ok, _ := resp["ok"].(bool); ok {
		t.Errorf("ok=true unexpected")
	}
	if resp["code"] != "live_ai_error" {
		t.Errorf("code: %v", resp["code"])
	}
	debug, _ := resp["debug"].(liveAIDebugInfo)
	if debug.Provider != "openai-compatible" {
		t.Errorf("debug not present")
	}

	// Plain error → no debug field.
	resp2 := liveAIErrorResponse(errors.New("plain"))
	if _, has := resp2["debug"]; has {
		t.Errorf("plain error should not include debug")
	}
}

// 11. orchestrateMode: single mode → no switch.
func TestOrchestrateMode_SingleMode_NoSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	current := modeRow{ID: 1, Name: "Solo", OrchestratorCheckInterval: 5}
	got, switched, err := h.orchestrateMode(context.Background(), env.Pool, 1, 1, current, []int64{1})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if switched {
		t.Errorf("single mode should not switch")
	}
	if got.ID != current.ID {
		t.Errorf("got %d want %d", got.ID, current.ID)
	}
}

func TestOrchestrateMode_PromocodeSingleSelectionFallsBackToPromoModes(t *testing.T) {
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Найти причины проблемы"})
	m2 := f.CreateMode(TestModeOpts{Name: "Уточнить фразу"})
	sourceID := int64(814)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100, AccessType: "promocode", SourceID: &sourceID})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100, AccessType: "promocode", SourceID: &sourceID})

	dialog := f.CreateDialog(user.ID, m1.ID)
	_ = f.AppendMessage(dialog.ID, "user", "третий ход должен вызвать оркестратор")

	fake := fakeVseGPT(t, http.StatusOK, `{"modeId":`+itoa(m2.ID)+`,"reason":"promo set"}`)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	current := modeRow{ID: m1.ID, Name: "Найти причины проблемы", OrchestratorCheckInterval: 1}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want mode %d switched", next.ID, switched, m2.ID)
	}
}

func TestOrchestrateMode_ManualSingleSelectionStaysSingle(t *testing.T) {
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Ручной режим 1"})
	m2 := f.CreateMode(TestModeOpts{Name: "Ручной режим 2"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100, AccessType: "manual"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100, AccessType: "manual"})

	dialog := f.CreateDialog(user.ID, m1.ID)
	_ = f.AppendMessage(dialog.ID, "user", "одиночный ручной выбор")

	h := Handler{DB: env.Pool}
	current := modeRow{ID: m1.ID, Name: "Ручной режим 1", OrchestratorCheckInterval: 1}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if switched || next.ID != m1.ID {
		t.Fatalf("next=%d switched=%v, want no switch", next.ID, switched)
	}
}

// 12. orchestrateMode: current not in knowledge IDs → no switch.
func TestOrchestrateMode_CurrentNotInList_NoSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	current := modeRow{ID: 99, Name: "X", OrchestratorCheckInterval: 5}
	got, switched, err := h.orchestrateMode(context.Background(), env.Pool, 1, 1, current, []int64{1, 2})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if switched {
		t.Errorf("current=99 not in [1,2] → should not switch")
	}
	if got.ID != 99 {
		t.Errorf("got %d want 99", got.ID)
	}
}

// 13. orchestrateMode: userMessages=0 → no switch.
func TestOrchestrateMode_NoMessages_NoSwitch(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{})
	m2 := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, m1.ID)
	// 0 user messages in dialogs_messages

	current := modeRow{ID: m1.ID, Name: "m1", OrchestratorCheckInterval: 5}
	_, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if switched {
		t.Errorf("0 messages: should not switch")
	}
}

// 14. orchestrateMode: AI picks another mode → switches.
func TestOrchestrateMode_AISwitches(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Mode1"})
	m2 := f.CreateMode(TestModeOpts{Name: "Mode2"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	// Exactly 5 user messages → triggers orchestration (interval=5)
	for i := 0; i < 5; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "q")
	}

	// Fake AI answers with JSON picking m2
	fake := fakeVseGPT(t, 200, `{"modeId":`+itoa(m2.ID)+`,"reason":"switch"}`)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	current := modeRow{ID: m1.ID, Name: "Mode1", AIModel: "gpt-4o-mini", OrchestratorCheckInterval: 5}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched {
		t.Errorf("AI picked m2: should switch")
	}
	if next.ID != m2.ID {
		t.Errorf("next mode: got %d want %d", next.ID, m2.ID)
	}
}

func TestOrchestrateMode_CountsMessagesAfterLastSwitchTrace(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Уточнить фразу"})
	m2 := f.CreateMode(TestModeOpts{Name: "Переговоры"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	for i := 0; i < 8; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "old q")
	}
	if _, err := insertModeSwitchTrace(context.Background(), env.Pool, dialog.ID, "Старый режим", "Уточнить фразу", true); err != nil {
		t.Fatalf("insert switch trace: %v", err)
	}
	for i := 0; i < 3; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "new q")
	}

	h := Handler{DB: env.Pool}
	current := modeRow{ID: m1.ID, Name: "Уточнить фразу", AIModel: "gpt-4o-mini", OrchestratorCheckInterval: 4}
	_, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err before interval: %v", err)
	}
	if switched {
		t.Fatalf("старые сообщения до switch trace не должны запускать оркестратор")
	}

	_ = f.AppendMessage(dialog.ID, "user", "new q 4")
	fake := fakeVseGPT(t, http.StatusOK, `{"modeId":`+itoa(m2.ID)+`,"reason":"switch"}`)
	h = Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err on interval: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want mode %d switched", next.ID, switched, m2.ID)
	}
}

func TestOrchestrateMode_UsesConfiguredOrchestrationModel(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Mode1", AIModel: "openai/current-mode-model"})
	m2 := f.CreateMode(TestModeOpts{Name: "Mode2"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	for i := 0; i < 5; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "q")
	}

	payloads := make(chan openAIChatRequest, 1)
	fake := fakeVseGPTCapturingRequest(t, payloads, `{"modeId":`+itoa(m2.ID)+`,"reason":"switch"}`)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	upsertSetting(t, h, "ai_orchestration_model", "openai/orchestration-audit")

	current := modeRow{ID: m1.ID, Name: "Mode1", AIModel: "openai/current-mode-model", OrchestratorCheckInterval: 5}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want mode %d switched", next.ID, switched, m2.ID)
	}
	payload := <-payloads
	if payload.Model != "openai/orchestration-audit" {
		t.Fatalf("orchestration model=%q want configured openai/orchestration-audit", payload.Model)
	}
	assertOrchestrationResponseFormat(t, payload, m1.ID, m2.ID)
}

func TestOrchestrateMode_DefaultModelDoesNotFallBackToCurrentMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Mode1", AIModel: "openai/current-mode-model"})
	m2 := f.CreateMode(TestModeOpts{Name: "Mode2"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	for i := 0; i < 5; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "q")
	}

	payloads := make(chan openAIChatRequest, 1)
	fake := fakeVseGPTCapturingRequest(t, payloads, `{"modeId":`+itoa(m2.ID)+`,"reason":"switch"}`)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	current := modeRow{ID: m1.ID, Name: "Mode1", AIModel: "openai/current-mode-model", OrchestratorCheckInterval: 5}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want mode %d switched", next.ID, switched, m2.ID)
	}
	payload := <-payloads
	if payload.Model != defaultAIOrchestrationModel {
		t.Fatalf("default orchestration model=%q want %q", payload.Model, defaultAIOrchestrationModel)
	}
	if payload.Temperature != defaultAIOrchestrationTemperature {
		t.Fatalf("default orchestration temperature=%v want %v", payload.Temperature, defaultAIOrchestrationTemperature)
	}
	if len(payload.Messages) == 0 || !strings.Contains(payload.Messages[0]["content"], "modeId") || !strings.Contains(payload.Messages[0]["content"], "criteria каждого режима") {
		t.Fatalf("default orchestration prompt missing routing guardrails: %#v", payload.Messages)
	}
	assertOrchestrationResponseFormat(t, payload, m1.ID, m2.ID)
}

func TestOrchestrateMode_ReplacesWeakConfiguredPrompt(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Найти причины проблемы"})
	m2 := f.CreateMode(TestModeOpts{Name: "Снизить тревогу"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	for i := 0; i < 5; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "q")
	}

	payloads := make(chan openAIChatRequest, 1)
	fake := fakeVseGPTCapturingRequest(t, payloads, `{"modeId":`+itoa(m1.ID)+`,"reason":"keep"}`)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	upsertSetting(t, h, "orchestration_prompt_default", "Ты оркестратор режимов. Смотри на критерии")

	current := modeRow{ID: m1.ID, Name: "Найти причины проблемы", AIModel: "openai/current-mode-model", OrchestratorCheckInterval: 5}
	_, _, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	payload := <-payloads
	if len(payload.Messages) == 0 {
		t.Fatalf("missing orchestration messages")
	}
	systemPrompt := payload.Messages[0]["content"]
	if !strings.Contains(systemPrompt, "modeId") || !strings.Contains(systemPrompt, "criteria каждого режима") {
		t.Fatalf("weak prompt was not replaced with technical fallback: %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "снять аффект") || strings.Contains(systemPrompt, "системного анализа") {
		t.Fatalf("technical fallback must not duplicate mode criteria: %q", systemPrompt)
	}
}

func TestOrchestrateMode_LogsDecisionPayloadAndLastUserMessage(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Снизить тревогу"})
	m2 := f.CreateMode(TestModeOpts{Name: "Найти решение"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	_ = f.AppendMessage(dialog.ID, "user", "мне было страшно выбирать задачу")
	_ = f.AppendMessage(dialog.ID, "assistant", "сначала стабилизируем состояние")
	_ = f.AppendMessage(dialog.ID, "user", "я успокоилась, теперь нужны идеи для усиления эффекта")

	fake := fakeVseGPT(t, http.StatusOK, `{"modeId":`+itoa(m2.ID)+`,"reason":"последний запрос про идеи"}`)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	current := modeRow{ID: m1.ID, Name: "Снизить тревогу", OrchestratorCheckInterval: 2}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want mode %d switched", next.ID, switched, m2.ID)
	}

	var decision, inputUser, lastUser, rawAnswer string
	var selectedModeID, appliedModeID int64
	var candidateCount int
	err = env.Pool.QueryRow(context.Background(), `
		select decision, selected_mode_id, applied_mode_id, candidate_count, input_user, last_user_message, raw_answer
		from orchestration_decision_logs
		where dialog_id = $1
		order by id desc
		limit 1`, dialog.ID).Scan(&decision, &selectedModeID, &appliedModeID, &candidateCount, &inputUser, &lastUser, &rawAnswer)
	if err != nil {
		t.Fatalf("read orchestration log: %v", err)
	}
	if decision != "switched" || selectedModeID != m2.ID || appliedModeID != m2.ID {
		t.Fatalf("log decision=%q selected=%d applied=%d, want switched to %d", decision, selectedModeID, appliedModeID, m2.ID)
	}
	if candidateCount != 2 {
		t.Fatalf("candidate_count=%d want 2", candidateCount)
	}
	if !strings.Contains(inputUser, "Последнее сообщение пользователя") || !strings.Contains(inputUser, "главный вес") {
		t.Fatalf("orchestration input does not emphasize latest user message: %q", inputUser)
	}
	if lastUser != "я успокоилась, теперь нужны идеи для усиления эффекта" {
		t.Fatalf("last_user_message=%q", lastUser)
	}
	if !strings.Contains(rawAnswer, "последний запрос про идеи") {
		t.Fatalf("raw_answer=%q", rawAnswer)
	}
}

func TestOrchestrateMode_UsesConfiguredHistoryWindow(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Снизить тревогу"})
	m2 := f.CreateMode(TestModeOpts{Name: "Найти решение"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	_ = f.AppendMessage(dialog.ID, "user", "старый контекст вне окна")
	_ = f.AppendMessage(dialog.ID, "assistant", "старый ответ вне окна")
	_ = f.AppendMessage(dialog.ID, "user", "средний пользователь тоже вне окна")
	_ = f.AppendMessage(dialog.ID, "assistant", "свежий ответ внутри окна")
	_ = f.AppendMessage(dialog.ID, "user", "свежий запрос внутри окна")

	fake := fakeVseGPT(t, http.StatusOK, `{"modeId":`+itoa(m2.ID)+`,"reason":"switch"}`)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}
	upsertSetting(t, h, "ai_orchestration_history_limit", "2")

	current := modeRow{ID: m1.ID, Name: "Снизить тревогу", OrchestratorCheckInterval: 3}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want mode %d switched", next.ID, switched, m2.ID)
	}

	var inputUser, lastUser string
	err = env.Pool.QueryRow(context.Background(), `
		select input_user, last_user_message
		from orchestration_decision_logs
		where dialog_id = $1
		order by id desc
		limit 1`, dialog.ID).Scan(&inputUser, &lastUser)
	if err != nil {
		t.Fatalf("read orchestration log: %v", err)
	}
	if lastUser != "свежий запрос внутри окна" {
		t.Fatalf("last_user_message=%q", lastUser)
	}
	for _, forbidden := range []string{"старый контекст вне окна", "старый ответ вне окна", "средний пользователь тоже вне окна"} {
		if strings.Contains(inputUser, forbidden) {
			t.Fatalf("orchestration input contains message outside configured window %q: %q", forbidden, inputUser)
		}
	}
	for _, required := range []string{"свежий ответ внутри окна", "свежий запрос внутри окна"} {
		if !strings.Contains(inputUser, required) {
			t.Fatalf("orchestration input missing message inside configured window %q: %q", required, inputUser)
		}
	}
}

func TestOrchestrationHistoryLimit_DefaultsAndClamps(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	if got := h.orchestrationHistoryLimit(context.Background()); got != defaultOrchestrationHistoryLimit {
		t.Fatalf("default orchestration history limit=%d want %d", got, defaultOrchestrationHistoryLimit)
	}
	upsertSetting(t, h, "ai_orchestration_history_limit", "0")
	if got := h.orchestrationHistoryLimit(context.Background()); got != minOrchestrationHistoryLimit {
		t.Fatalf("min-clamped orchestration history limit=%d want %d", got, minOrchestrationHistoryLimit)
	}
	upsertSetting(t, h, "ai_orchestration_history_limit", "999")
	if got := h.orchestrationHistoryLimit(context.Background()); got != maxOrchestrationHistoryLimit {
		t.Fatalf("max-clamped orchestration history limit=%d want %d", got, maxOrchestrationHistoryLimit)
	}
	upsertSetting(t, h, "ai_orchestration_history_limit", "not-a-number")
	if got := h.orchestrationHistoryLimit(context.Background()); got != defaultOrchestrationHistoryLimit {
		t.Fatalf("invalid orchestration history limit=%d want default %d", got, defaultOrchestrationHistoryLimit)
	}
}

func TestOrchestrateMode_RepeatedKeepRetriesWithoutCurrentMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Снизить тревогу"})
	m2 := f.CreateMode(TestModeOpts{Name: "Найти решение"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	_ = f.AppendMessage(dialog.ID, "user", "я успокоилась, нужны идеи")
	if _, err := env.Pool.Exec(context.Background(), `
		insert into orchestration_decision_logs (
			user_id, dialog_id, current_mode_id, selected_mode_id, applied_mode_id,
			candidate_mode_ids, candidate_count, decision
		) values ($1, $2, $3, $3, $3, $4, 2, 'kept_current')`,
		user.ID, dialog.ID, m1.ID, []int64{m1.ID, m2.ID}); err != nil {
		t.Fatalf("seed previous keep log: %v", err)
	}

	payloads := make(chan openAIChatRequest, 2)
	fake := fakeVseGPTSequenceCapturingRequests(t, payloads,
		`{"modeId":`+itoa(m1.ID)+`,"reason":"оставить тревогу"}`,
		`{"modeId":`+itoa(m2.ID)+`,"reason":"второй раз выбрать альтернативу"}`,
	)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	current := modeRow{ID: m1.ID, Name: "Снизить тревогу", OrchestratorCheckInterval: 1}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want forced switch to %d", next.ID, switched, m2.ID)
	}

	first := <-payloads
	second := <-payloads
	assertOrchestrationResponseFormat(t, first, m1.ID, m2.ID)
	assertOrchestrationResponseFormat(t, second, m2.ID)
	if len(second.Messages) < 2 || !strings.Contains(second.Messages[1]["content"], "не выбирай текущий режим") {
		t.Fatalf("retry prompt does not exclude current mode: %#v", second.Messages)
	}

	var decision, retryRaw string
	var retryWithoutCurrent bool
	var selectedModeID, appliedModeID int64
	err = env.Pool.QueryRow(context.Background(), `
		select decision, selected_mode_id, applied_mode_id, retry_raw_answer, retry_without_current
		from orchestration_decision_logs
		where dialog_id = $1
		order by id desc
		limit 1`, dialog.ID).Scan(&decision, &selectedModeID, &appliedModeID, &retryRaw, &retryWithoutCurrent)
	if err != nil {
		t.Fatalf("read orchestration log: %v", err)
	}
	if decision != "forced_switch_after_repeat_keep" || selectedModeID != m2.ID || appliedModeID != m2.ID || !retryWithoutCurrent {
		t.Fatalf("decision=%q selected=%d applied=%d retry=%v, want forced switch to %d", decision, selectedModeID, appliedModeID, retryWithoutCurrent, m2.ID)
	}
	if !strings.Contains(retryRaw, "второй раз выбрать альтернативу") {
		t.Fatalf("retry_raw_answer=%q", retryRaw)
	}
}

func TestInsertOrchestrationDecisionLog_RotatesOldAndCapsRows(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "Снизить тревогу"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 100})
	dialog := f.CreateDialog(user.ID, mode.ID)

	if _, err := env.Pool.Exec(context.Background(), `
		insert into orchestration_decision_logs (
			user_id, dialog_id, current_mode_id, applied_mode_id, decision, created_at
		)
		values ($1, $2, $3, $3, 'old_row', now() - interval '25 hours')`,
		user.ID, dialog.ID, mode.ID); err != nil {
		t.Fatalf("seed old log: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `
		insert into orchestration_decision_logs (
			user_id, dialog_id, current_mode_id, applied_mode_id, decision, created_at
		)
		select $1, $2, $3, $3, 'recent_row', now() - (g || ' seconds')::interval
		from generate_series(1, 5000) as g`,
		user.ID, dialog.ID, mode.ID); err != nil {
		t.Fatalf("seed recent logs: %v", err)
	}

	insertOrchestrationDecisionLog(context.Background(), env.Pool, orchestrationDecisionLogEntry{
		UserID:        user.ID,
		DialogID:      dialog.ID,
		CurrentModeID: mode.ID,
		AppliedModeID: mode.ID,
		Decision:      "kept_current",
	})

	var total, oldRows int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from orchestration_decision_logs`).Scan(&total); err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from orchestration_decision_logs where decision = 'old_row'`).Scan(&oldRows); err != nil {
		t.Fatalf("count old logs: %v", err)
	}
	if total > 5000 {
		t.Fatalf("orchestration log rows=%d want <=5000", total)
	}
	if oldRows != 0 {
		t.Fatalf("old rows=%d want 0", oldRows)
	}
}

func TestOrchestrateMode_FallsBackWhenStructuredOutputsUnsupported(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	m1 := f.CreateMode(TestModeOpts{Name: "Mode1"})
	m2 := f.CreateMode(TestModeOpts{Name: "Mode2"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m1.ID, DailyMessageLimit: 100})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: m2.ID, DailyMessageLimit: 100})

	dialog := f.CreateDialog(user.ID, m1.ID)
	for i := 0; i < 5; i++ {
		_ = f.AppendMessage(dialog.ID, "user", "q")
	}

	payloads := make(chan openAIChatRequest, 2)
	fake := fakeVseGPTStructuredFallback(t, payloads, m2.ID)
	h := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	}

	current := modeRow{ID: m1.ID, Name: "Mode1", AIModel: "openai/current-mode-model", OrchestratorCheckInterval: 5}
	next, switched, err := h.orchestrateMode(context.Background(), env.Pool, user.ID, dialog.ID, current, []int64{m1.ID, m2.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !switched || next.ID != m2.ID {
		t.Fatalf("next=%d switched=%v, want mode %d switched", next.ID, switched, m2.ID)
	}
	first := <-payloads
	second := <-payloads
	if first.ResponseFormat == nil {
		t.Fatalf("first orchestration request missing response_format")
	}
	if second.ResponseFormat != nil {
		t.Fatalf("fallback orchestration request should omit response_format: %#v", second.ResponseFormat)
	}
}

func fakeVseGPTCapturingRequest(t *testing.T, payloads chan<- openAIChatRequest, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		var payload openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		payloads <- payload
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
				"cost":              0.001,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeVseGPTStructuredFallback(t *testing.T, payloads chan<- openAIChatRequest, modeID int64) *httptest.Server {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		payloads <- payload
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"response_format json_schema unsupported"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"modeId":` + itoa(modeID) + `,"reason":"switch"}`}},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
				"cost":              0.001,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeVseGPTSequenceCapturingRequests(t *testing.T, payloads chan<- openAIChatRequest, contents ...string) *httptest.Server {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		var payload openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		payloads <- payload
		idx := int(calls.Add(1)) - 1
		if idx >= len(contents) {
			idx = len(contents) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": contents[idx]}},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
				"cost":              0.001,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func assertOrchestrationResponseFormat(t *testing.T, payload openAIChatRequest, allowed ...int64) {
	t.Helper()
	raw, ok := payload.ResponseFormat.(map[string]any)
	if !ok {
		t.Fatalf("response_format=%T %#v, want object", payload.ResponseFormat, payload.ResponseFormat)
	}
	if raw["type"] != "json_schema" {
		t.Fatalf("response_format.type=%v want json_schema", raw["type"])
	}
	jsonSchema, ok := raw["json_schema"].(map[string]any)
	if !ok {
		t.Fatalf("response_format.json_schema=%T %#v", raw["json_schema"], raw["json_schema"])
	}
	if jsonSchema["strict"] != true {
		t.Fatalf("response_format.json_schema.strict=%v want true", jsonSchema["strict"])
	}
	schema, ok := jsonSchema["schema"].(map[string]any)
	if !ok {
		t.Fatalf("response_format schema=%T %#v", jsonSchema["schema"], jsonSchema["schema"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties=%T %#v", schema["properties"], schema["properties"])
	}
	modeID, ok := properties["modeId"].(map[string]any)
	if !ok {
		t.Fatalf("modeId schema=%T %#v", properties["modeId"], properties["modeId"])
	}
	enum, ok := modeID["enum"].([]any)
	if !ok {
		t.Fatalf("modeId enum=%T %#v", modeID["enum"], modeID["enum"])
	}
	if len(enum) != len(allowed) {
		t.Fatalf("modeId enum=%v want %v", enum, allowed)
	}
	for i, want := range allowed {
		got, ok := enum[i].(float64)
		if !ok || int64(got) != want {
			t.Fatalf("modeId enum[%d]=%T %v want %d", i, enum[i], enum[i], want)
		}
	}
}

// 15. orchestrationCandidates: filters by active access.
func TestOrchestrationCandidates_FiltersByAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	withAccess := f.CreateMode(TestModeOpts{Name: "Allowed"})
	noAccess := f.CreateMode(TestModeOpts{Name: "NotAllowed"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: withAccess.ID, DailyMessageLimit: 100})

	cands, err := h.orchestrationCandidates(context.Background(), env.Pool, user.ID, []int64{withAccess.ID, noAccess.ID})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	found := false
	for _, c := range cands {
		if c.ID == withAccess.ID {
			found = true
		}
		if c.ID == noAccess.ID {
			t.Errorf("leaked mode without access")
		}
	}
	if !found {
		t.Errorf("allowed mode missing: %v", cands)
	}
}

// 16. parseOrchestratorModeID: extracts modeId from JSON answer.
func TestParseOrchestratorModeID(t *testing.T) {
	t.Parallel()
	allowed := []int64{10, 20, 30}

	cases := []struct {
		answer string
		want   int64
	}{
		{`{"modeId":20,"reason":"x"}`, 20},
		{"text before {\"modeId\":30} text after", 30},
		{`{"modeId":999}`, 0},                         // not in allowed
		{`no json here, but mentions 10 somehow`, 10}, // fallback by substring
		{"", 0},
		{`{"unrelated":true}`, 0},
	}
	for _, c := range cases {
		got := parseOrchestratorModeID(c.answer, allowed)
		if got != c.want {
			t.Errorf("answer %q: got %d want %d", c.answer, got, c.want)
		}
	}
}

// 17. idInList helper.
func TestIDInList(t *testing.T) {
	t.Parallel()
	if !idInList(5, []int64{1, 5, 7}) {
		t.Errorf("5 in [1,5,7]: false")
	}
	if idInList(9, []int64{1, 5, 7}) {
		t.Errorf("9 in [1,5,7]: true")
	}
	if idInList(1, nil) {
		t.Errorf("1 in nil: true")
	}
}

// 18. messagesText format.
func TestMessagesText_Format(t *testing.T) {
	t.Parallel()
	got := messagesText([]map[string]string{
		{"role": "system", "content": "S"},
		{"role": "user", "content": "U"},
	})
	if !strings.Contains(got, "system:S") || !strings.Contains(got, "user:U") {
		t.Errorf("format: %q", got)
	}
	if got != "system:S\nuser:U" {
		t.Errorf("exact format: %q", got)
	}
}

// 19. nullableCost / firstPositiveFloat / effectiveLiveAIModel / openAICompatibleModel —
// extra branches not previously covered.
func TestEffectiveLiveAIModel_TrimsAndFallsBack(t *testing.T) {
	t.Parallel()
	h := Handler{}
	if got := h.effectiveLiveAIModel("  "); got != "gpt-4o-mini" {
		t.Errorf("whitespace fallback: %q", got)
	}
	if got := h.effectiveLiveAIModel("custom-model"); got != "custom-model" {
		t.Errorf("explicit: %q", got)
	}
}

func TestOpenAICompatibleModel_ContainsSlash_NoChange(t *testing.T) {
	t.Parallel()
	h := Handler{OpenAIBaseURL: "https://api.vsegpt.ru/v1"}
	if got := h.openAICompatibleModel("provider/model"); got != "provider/model" {
		t.Errorf("slash should preserve: %q", got)
	}
}

func TestOpenAICompatibleModel_O1Prefix(t *testing.T) {
	t.Parallel()
	h := Handler{OpenAIBaseURL: "https://api.vsegpt.ru/v1"}
	if got := h.openAICompatibleModel("o1-preview"); got != "openai/o1-preview" {
		t.Errorf("o1 should be prefixed: %q", got)
	}
}

// 20. configuredAIModel with system_settings override.
func TestConfiguredAIModel_SettingsOverride(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	// Insert override
	_, _ = env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('test_model_key', 'custom-override-model'), ('test_temp_key', '0.8') on conflict (key) do update set value=excluded.value`)

	model, temp := h.configuredAIModel(context.Background(),
		"test_model_key", "test_temp_key", "fallback-model", 0.3)
	if model != "custom-override-model" {
		t.Errorf("model override: %q", model)
	}
	if temp != 0.8 {
		t.Errorf("temp override: %v want 0.8", temp)
	}
}

func TestConfiguredAIModel_ClampsTemperature(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	h := Handler{DB: env.Pool}

	_, _ = env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('clamp_temp_key', '5.0') on conflict (key) do update set value=excluded.value`)

	_, temp := h.configuredAIModel(context.Background(), "absent_model_key", "clamp_temp_key", "m", 0.3)
	if temp != 2 {
		t.Errorf("temp should clamp to 2, got %v", temp)
	}

	_, _ = env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('clamp_temp_neg', '-1.0') on conflict (key) do update set value=excluded.value`)
	_, temp = h.configuredAIModel(context.Background(), "absent", "clamp_temp_neg", "m", 0.3)
	if temp != 0 {
		t.Errorf("temp should clamp to 0, got %v", temp)
	}
}
