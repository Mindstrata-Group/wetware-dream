//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// ============================================================================
// PUBLIC DEMO MODES (cached, TTL = 5min)
// ============================================================================

// TestPublicDemoModes_ReturnsList: GET /api/public/demo-modes → 200 + list.
func TestPublicDemoModes_ReturnsList(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ts.Handler.clearPublicDemoModesCache() // isolate from other tests

	status, body := httpJSON(t, ts, "GET", "/api/public/demo-modes", nil)
	if status != http.StatusOK {
		t.Fatalf("status: %d body=%v", status, body)
	}
	// The list may be empty if there are no modes in the DB: what matters is the structure.
	if _, ok := body["modes"].([]any); !ok {
		t.Errorf("modes field missing or wrong type: %v", body)
	}
}

// TestPublicDemoModes_CacheHit: a repeated request returns the cache (no new DB query).
// Checked indirectly: after clearCache → request → inspect the populated cache,
// then a second request: the cache is still valid.
func TestPublicDemoModes_CacheHit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ts.Handler.clearPublicDemoModesCache()

	// First request → must fill the cache.
	_, _ = httpJSON(t, ts, "GET", "/api/public/demo-modes", nil)

	c := &ts.Handler.c.publicDemoModes
	c.RLock()
	cachedAt := c.expiresAt
	cachedModes := c.modes
	c.RUnlock()

	if cachedModes == nil {
		t.Fatalf("cache not populated after first request")
	}
	if cachedAt.Before(time.Now()) {
		t.Fatalf("cache expiry in past: %v", cachedAt)
	}
	// TTL must be about 5 minutes.
	if cachedAt.Sub(time.Now()) > publicDemoModesCacheTTL+5*time.Second {
		t.Errorf("cache TTL too large: %v", cachedAt.Sub(time.Now()))
	}
	if cachedAt.Sub(time.Now()) < publicDemoModesCacheTTL-30*time.Second {
		t.Errorf("cache TTL too small: %v", cachedAt.Sub(time.Now()))
	}

	// Second request: the cache must stay populated.
	_, _ = httpJSON(t, ts, "GET", "/api/public/demo-modes", nil)
	c.RLock()
	if c.modes == nil {
		t.Errorf("cache cleared between requests")
	}
	c.RUnlock()
}

// TestPublicDemoModes_ClearCacheInvalidates: clearPublicDemoModesCache() → the next request goes to the DB.
func TestPublicDemoModes_ClearCacheInvalidates(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ts.Handler.clearPublicDemoModesCache()

	// Fill the cache
	_, _ = httpJSON(t, ts, "GET", "/api/public/demo-modes", nil)
	c := &ts.Handler.c.publicDemoModes
	c.RLock()
	populated := c.modes != nil
	c.RUnlock()
	if !populated {
		t.Fatalf("setup: cache not populated")
	}

	ts.Handler.clearPublicDemoModesCache()

	c.RLock()
	cleared := c.modes == nil
	c.RUnlock()
	if !cleared {
		t.Errorf("clearPublicDemoModesCache didn't clear")
	}
}

// TestPublicDemoModes_RejectsNonGET.
func TestPublicDemoModes_RejectsNonGET(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	status, _ := httpJSON(t, ts, "POST", "/api/public/demo-modes", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", status)
	}
}

// TestPublicDemoModes_RequiresDB: if h.DB is nil → 503.
func TestPublicDemoModes_RequiresDB(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	_ = env
	// Handler without a DB: built by hand.
	handler := Handler{DB: nil, c: newHandlerCaches()}
	mux := NewRouter(handler, []string{"*"})
	// Use http.NewRequest directly: no TestServer needed either.
	req, _ := http.NewRequest("GET", "/api/public/demo-modes", nil)
	rr := newRecorder()
	mux.ServeHTTP(rr, req)
	if rr.code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 without DB, got %d", rr.code)
	}
}

func TestPublicTariffs_ReturnsOnlyStorefrontTariffsWithModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{Name: "Режим витрины", WelcomeMessage: "Приветствие режима для подсказки"})
	hiddenMode := f.CreateMode(TestModeOpts{Name: "Скрытый режим"})
	if _, err := env.Pool.Exec(context.Background(), `update modes set hidden_at = now() where id = $1`, hiddenMode.ID); err != nil {
		t.Fatalf("hide mode: %v", err)
	}

	var groupID int64
	if err := env.Pool.QueryRow(context.Background(), `
		insert into tariff_groups (name, description, sort_order, available_for_subscription, created_at, updated_at)
		values ('Публичная группа', '', 10, true, now(), now())
		returning id`).Scan(&groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	var hiddenGroupID int64
	if err := env.Pool.QueryRow(context.Background(), `
		insert into tariff_groups (name, description, sort_order, available_for_subscription, created_at, updated_at)
		values ('Скрытая группа', '', 20, false, now(), now())
		returning id`).Scan(&hiddenGroupID); err != nil {
		t.Fatalf("seed hidden group: %v", err)
	}

	var publicTariffID, promoTariffID, hiddenGroupTariffID int64
	if err := env.Pool.QueryRow(context.Background(), `
		insert into tariffs (name, description, monthly_price, daily_message_limit, limit_type, group_id, available_for_subscription, tariff_type, created_at, updated_at)
		values ('Публичный тариф', 'Для витрины', 1200, 70, 'shared', $1, true, 'regular', now(), now())
		returning id`, groupID).Scan(&publicTariffID); err != nil {
		t.Fatalf("seed public tariff: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(), `
		insert into tariffs (name, description, monthly_price, daily_message_limit, limit_type, group_id, available_for_subscription, tariff_type, created_at, updated_at)
		values ('Промо тариф', '', 900, 50, 'shared', $1, true, 'promo', now(), now())
		returning id`, groupID).Scan(&promoTariffID); err != nil {
		t.Fatalf("seed promo tariff: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(), `
		insert into tariffs (name, description, monthly_price, daily_message_limit, limit_type, group_id, available_for_subscription, tariff_type, created_at, updated_at)
		values ('Тариф скрытой группы', '', 1500, 50, 'shared', $1, true, 'regular', now(), now())
		returning id`, hiddenGroupID).Scan(&hiddenGroupTariffID); err != nil {
		t.Fatalf("seed hidden group tariff: %v", err)
	}

	if _, err := env.Pool.Exec(context.Background(), `
		insert into tariff_mode (tariff_id, mode_id, daily_message_limit, created_at)
		values ($1, $2, 70, now()), ($1, $3, 70, now()), ($4, $2, 50, now()), ($5, $2, 50, now())`,
		publicTariffID, mode.ID, hiddenMode.ID, promoTariffID, hiddenGroupTariffID); err != nil {
		t.Fatalf("seed tariff modes: %v", err)
	}

	status, body := httpJSON(t, ts, "GET", "/api/public/tariffs", nil)
	if status != http.StatusOK {
		t.Fatalf("status: %d body=%v", status, body)
	}
	tariffs, ok := body["tariffs"].([]any)
	if !ok || len(tariffs) == 0 {
		t.Fatalf("tariffs missing: %v", body)
	}

	found := false
	for _, raw := range tariffs {
		item, _ := raw.(map[string]any)
		if item["name"] == "Публичный тариф" {
			found = true
			if item["monthlyPrice"].(float64) != 1200 {
				t.Fatalf("monthlyPrice=%v", item["monthlyPrice"])
			}
			modes, _ := item["modes"].([]any)
			if len(modes) != 1 {
				t.Fatalf("modes=%v, want only visible mode", modes)
			}
			modeItem, _ := modes[0].(map[string]any)
			if modeItem["welcomeMessage"] != "Приветствие режима для подсказки" {
				t.Fatalf("welcomeMessage=%v", modeItem["welcomeMessage"])
			}
		}
		if item["name"] == "Промо тариф" || item["name"] == "Тариф скрытой группы" {
			t.Fatalf("private tariff leaked: %v", item)
		}
	}
	if !found {
		t.Fatalf("public tariff not found in %v", tariffs)
	}
}

// ============================================================================
// CHAT MODES (listModes — cached, listUserModes — per-user, no cache)
// ============================================================================

// TestChatModes_ListUserModes_FiltersByAccess: only active grants → visible.
func TestChatModes_ListUserModes_FiltersByAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool, c: newHandlerCaches()}

	user := f.CreateUser(TestUserOpts{})
	visibleMode := f.CreateMode(TestModeOpts{Name: "VisibleToUser"})
	hiddenMode := f.CreateMode(TestModeOpts{Name: "NotForUser"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: visibleMode.ID, DailyMessageLimit: 50})
	// hiddenMode: no grant, not visible

	modes, err := h.listUserModes(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("listUserModes: %v", err)
	}

	foundVisible, foundHidden := false, false
	for _, m := range modes {
		if m.ID == visibleMode.ID {
			foundVisible = true
		}
		if m.ID == hiddenMode.ID {
			foundHidden = true
		}
	}
	if !foundVisible {
		t.Errorf("visibleMode not in result")
	}
	if foundHidden {
		t.Errorf("hiddenMode leaked into result")
	}
}

// TestChatModes_ListUserModes_ExpiredAccess_Excluded.
func TestChatModes_ListUserModes_ExpiredAccess_Excluded(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool, c: newHandlerCaches()}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "Expired"})
	past := timeAgo(48)
	pastPast := timeAgo(72)
	f.GrantAccess(GrantAccessOpts{
		UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50,
		ActiveFrom: &pastPast, ActiveTo: &past,
	})

	modes, err := h.listUserModes(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("listUserModes: %v", err)
	}
	for _, m := range modes {
		if m.ID == mode.ID {
			t.Errorf("expired mode leaked: %v", m)
		}
	}
}

// TestChatModes_ListModes_CachePopulated.
func TestChatModes_ListModes_CachePopulated(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool, c: newHandlerCaches()}

	h.clearModesCache()
	_ = f.CreateMode(TestModeOpts{Name: "CacheTest"})

	_, err := h.listModes(context.Background())
	if err != nil {
		t.Fatalf("listModes: %v", err)
	}

	h.c.modes.RLock()
	cached := h.c.modes.modes
	expiresAt := h.c.modes.expiresAt
	h.c.modes.RUnlock()

	if cached == nil {
		t.Fatalf("modesCache not populated")
	}
	if expiresAt.Before(time.Now()) {
		t.Errorf("modesCache expiry in past")
	}
}

// TestChatModes_ListModes_HiddenExcluded: hidden_at IS NOT NULL → not in the listing.
func TestChatModes_ListModes_HiddenExcluded(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool, c: newHandlerCaches()}

	visible := f.CreateMode(TestModeOpts{Name: "VisibleListMode"})
	hidden := f.CreateMode(TestModeOpts{Name: "HiddenListMode"})

	// Hide one mode
	_, err := env.Pool.Exec(context.Background(),
		`update modes set hidden_at = now() where id = $1`, hidden.ID)
	if err != nil {
		t.Fatalf("hide: %v", err)
	}

	h.clearModesCache()
	modes, err := h.listModes(context.Background())
	if err != nil {
		t.Fatalf("listModes: %v", err)
	}
	foundVisible, foundHidden := false, false
	for _, m := range modes {
		if m.ID == visible.ID {
			foundVisible = true
		}
		if m.ID == hidden.ID {
			foundHidden = true
		}
	}
	if !foundVisible {
		t.Errorf("visible mode missing")
	}
	if foundHidden {
		t.Errorf("hidden mode leaked")
	}
}

// TestChatModes_ClearModesCache_Invalidates.
func TestChatModes_ClearModesCache_Invalidates(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool, c: newHandlerCaches()}

	_ = f.CreateMode(TestModeOpts{})
	h.clearModesCache()
	_, _ = h.listModes(context.Background())

	h.c.modes.RLock()
	populated := h.c.modes.modes != nil
	h.c.modes.RUnlock()
	if !populated {
		t.Fatalf("setup: cache not populated")
	}

	h.clearModesCache()
	h.c.modes.RLock()
	cleared := h.c.modes.modes == nil
	h.c.modes.RUnlock()
	if !cleared {
		t.Errorf("clearModesCache didn't clear")
	}
}

// ============================================================================
// Tiny http.ResponseWriter recorder (avoid import httptest just for one test)
// ============================================================================

type tinyRecorder struct {
	code int
	hdr  http.Header
	body []byte
}

func newRecorder() *tinyRecorder            { return &tinyRecorder{code: 200, hdr: http.Header{}} }
func (r *tinyRecorder) Header() http.Header { return r.hdr }
func (r *tinyRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}
func (r *tinyRecorder) WriteHeader(c int) { r.code = c }
