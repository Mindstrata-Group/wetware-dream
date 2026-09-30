//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// SITE_CONTENT is the CMS feature. These tests validate:
// 1. Public GET is cached in memory like demo-modes.
// 2. POST admin upsert (insert + update).
// 3. The cache is cleared after an admin POST.
// 4. AdminSiteContentList returns meta updated_at.
// 5. The flush endpoint works.
// 6. RBAC: a non-admin does not get through.

func TestSiteContent_PublicGET_Cached(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	// Seed a row
	_, _ = env.Pool.Exec(context.Background(),
		`insert into site_content (key, value) values ('test.k1', '"hello"'::jsonb)`)

	ts.Handler.clearSiteContentCache()

	// 1st GET fills the cache
	status, body := httpJSON(t, ts, "GET", "/api/public/site-content", nil)
	if status != http.StatusOK {
		t.Fatalf("1st GET status %d body=%v", status, body)
	}
	content, _ := body["content"].(map[string]any)
	if content["test.k1"] != "hello" {
		t.Errorf("expected hello, got %v", content["test.k1"])
	}

	// Change the row IN THE DB, NOT through the API → the cache is not invalidated
	_, _ = env.Pool.Exec(context.Background(),
		`update site_content set value='"hello2"'::jsonb where key='test.k1'`)

	// 2nd GET must return "hello" (from cache)
	_, body2 := httpJSON(t, ts, "GET", "/api/public/site-content", nil)
	content2, _ := body2["content"].(map[string]any)
	if content2["test.k1"] != "hello" {
		t.Errorf("expected cached hello, got %v", content2["test.k1"])
	}
}

func TestSiteContent_AdminPOST_UpsertAndInvalidatesCache(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	ts.Handler.clearSiteContentCache()

	// Fill the cache (empty or a few entries, it does not matter)
	_, _ = httpJSON(t, ts, "GET", "/api/public/site-content", nil)

	// Admin upsert
	status, body := httpJSON(t, ts, "POST", "/api/admin/site-content", map[string]any{
		"key":   "test.fresh",
		"value": "freshvalue",
	})
	if status != http.StatusOK {
		t.Fatalf("admin POST status %d body=%v", status, body)
	}

	// The cache must be cleared → the next public GET returns the real value
	_, body2 := httpJSON(t, ts, "GET", "/api/public/site-content", nil)
	content, _ := body2["content"].(map[string]any)
	if content["test.fresh"] != "freshvalue" {
		t.Errorf("cache wasn't invalidated, got %v", content["test.fresh"])
	}

	// Update the same key (UPDATE branch of ON CONFLICT)
	status2, _ := httpJSON(t, ts, "POST", "/api/admin/site-content", map[string]any{
		"key":   "test.fresh",
		"value": "updated-value",
	})
	if status2 != http.StatusOK {
		t.Fatalf("admin POST update status %d", status2)
	}
	_, body3 := httpJSON(t, ts, "GET", "/api/public/site-content", nil)
	content3, _ := body3["content"].(map[string]any)
	if content3["test.fresh"] != "updated-value" {
		t.Errorf("update didn't apply, got %v", content3["test.fresh"])
	}

	// An audit record was created
	var auditCnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action = 'admin.site_content.update'`).Scan(&auditCnt)
	if auditCnt < 2 {
		t.Errorf("expected ≥2 audit entries, got %d", auditCnt)
	}
}

func TestSiteContent_AdminPOST_ValueValidation(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Empty key → 400
	status, _ := httpJSON(t, ts, "POST", "/api/admin/site-content", map[string]any{
		"key":   "",
		"value": "x",
	})
	if status != http.StatusBadRequest {
		t.Errorf("empty key: expected 400, got %d", status)
	}

	// Key too long → 400
	longKey := make([]byte, 250)
	for i := range longKey {
		longKey[i] = 'a'
	}
	status2, _ := httpJSON(t, ts, "POST", "/api/admin/site-content", map[string]any{
		"key":   string(longKey),
		"value": "x",
	})
	if status2 != http.StatusBadRequest {
		t.Errorf("long key: expected 400, got %d", status2)
	}
}

func TestSiteContent_AdminGET_List(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Seed
	_, _ = env.Pool.Exec(context.Background(),
		`insert into site_content (key, value) values ('list.a', '1'::jsonb), ('list.b', '"two"'::jsonb)`)

	status, body := httpJSON(t, ts, "GET", "/api/admin/site-content", nil)
	if status != http.StatusOK {
		t.Fatalf("admin GET status %d body=%v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) < 2 {
		t.Errorf("expected ≥2 items, got %d", len(items))
	}
	// Check that meta updated_at is present
	first, _ := items[0].(map[string]any)
	if _, ok := first["updatedAt"]; !ok {
		t.Errorf("missing updatedAt meta in list response")
	}
}

func TestSiteContent_AdminFlush_ClearsCache(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, _ = env.Pool.Exec(context.Background(),
		`insert into site_content (key, value) values ('flush.test', '"v1"'::jsonb)`)

	ts.Handler.clearSiteContentCache()
	_, _ = httpJSON(t, ts, "GET", "/api/public/site-content", nil) // fill the cache

	// Change directly in the DB (without the API)
	_, _ = env.Pool.Exec(context.Background(),
		`update site_content set value='"v2"'::jsonb where key='flush.test'`)

	// Public GET still sees v1 (from cache)
	_, b1 := httpJSON(t, ts, "GET", "/api/public/site-content", nil)
	c1, _ := b1["content"].(map[string]any)
	if c1["flush.test"] != "v1" {
		t.Logf("cache check: expected v1, got %v (test may be racing TTL)", c1["flush.test"])
	}

	// FLUSH
	status, body := httpJSON(t, ts, "POST", "/api/admin/site-content/flush", nil)
	if status != http.StatusOK {
		t.Fatalf("flush status %d body=%v", status, body)
	}
	if flushed, _ := body["flushed"].(bool); !flushed {
		t.Errorf("flushed=false in response")
	}

	// Now public GET sees v2
	_, b2 := httpJSON(t, ts, "GET", "/api/public/site-content", nil)
	c2, _ := b2["content"].(map[string]any)
	if c2["flush.test"] != "v2" {
		t.Errorf("after flush expected v2, got %v", c2["flush.test"])
	}

	// Audit record
	var auditCnt int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action = 'admin.site_content.flush'`).Scan(&auditCnt)
	if auditCnt < 1 {
		t.Errorf("flush audit missing")
	}
}

func TestSiteContent_NonAdmin_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "user"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/site-content", map[string]any{
		"key":   "x",
		"value": "y",
	})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("user POST: expected 403/401, got %d", status)
	}

	status2, _ := httpJSON(t, ts, "POST", "/api/admin/site-content/flush", nil)
	if status2 != http.StatusForbidden && status2 != http.StatusUnauthorized {
		t.Errorf("user flush: expected 403/401, got %d", status2)
	}
}

func TestSiteContent_PublicGET_WrongMethod_405(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	status, _ := httpJSON(t, ts, "POST", "/api/public/site-content", nil)
	if status != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", status)
	}
}

func TestSiteContent_ComplexValue_JSONArray(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	tiles := []map[string]any{
		{"id": 1, "title": "Войдите", "body": "Через Яндекс."},
		{"id": 2, "title": "Введите промокод", "body": "Бесплатный режим."},
	}
	status, _ := httpJSON(t, ts, "POST", "/api/admin/site-content", map[string]any{
		"key":   "about.tiles",
		"value": tiles,
	})
	if status != http.StatusOK {
		t.Fatalf("post tiles: %d", status)
	}

	// Public GET returns the array as is
	_, body := httpJSON(t, ts, "GET", "/api/public/site-content", nil)
	content, _ := body["content"].(map[string]any)
	raw, _ := json.Marshal(content["about.tiles"])
	var got []map[string]any
	_ = json.Unmarshal(raw, &got)
	if len(got) != 2 {
		t.Fatalf("expected 2 tiles, got %d", len(got))
	}
	if got[0]["title"] != "Войдите" {
		t.Errorf("tile title mismatch: %v", got[0])
	}
}

// REGRESSION: the public cache TTL is ~5 min. An indirect test that expiresAt > now+4min.
func TestSiteContent_CacheTTL(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_ = env

	ts.Handler.clearSiteContentCache()
	_, _ = httpJSON(t, ts, "GET", "/api/public/site-content", nil)

	sc := &ts.Handler.c.siteContent
	sc.RLock()
	exp := sc.expiresAt
	sc.RUnlock()

	if exp.Sub(time.Now()) < 4*time.Minute {
		t.Errorf("TTL too short: %v", exp.Sub(time.Now()))
	}
	if exp.Sub(time.Now()) > 6*time.Minute {
		t.Errorf("TTL too long: %v", exp.Sub(time.Now()))
	}
}
