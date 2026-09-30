//go:build integration

package httpapi

import (
	"context"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// Regression unit tests for the cache layer of admin endpoints (2026-05-31).
// They guard against accidentally removing clears or forgetting invalidation
// after mutations. We test the cache structures and clear functions themselves,
// without the HTTP layer (covered by breadth_coverage_integration_test.go).

// ============================================================================
// /api/admin/dialogs cache
// ============================================================================

func TestAdminDialogsCache_ClearResetsState(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	ts.Handler.c.adminDialogs.Lock()
	ts.Handler.c.adminDialogs.body = []byte("populated")
	ts.Handler.c.adminDialogs.expiresAt = time.Now().Add(time.Hour)
	ts.Handler.c.adminDialogs.Unlock()

	ts.Handler.c.adminDialogs.clear()

	ts.Handler.c.adminDialogs.Lock()
	body := ts.Handler.c.adminDialogs.body
	expiresAt := ts.Handler.c.adminDialogs.expiresAt
	ts.Handler.c.adminDialogs.Unlock()
	if body != nil {
		t.Errorf("body not cleared: %v", body)
	}
	if !expiresAt.IsZero() {
		t.Errorf("expiresAt not reset: %v", expiresAt)
	}
}

// ============================================================================
// /api/admin/users cache (keyed by q+limit+offset)
// ============================================================================

func TestAdminUsersListCache_ClearEmptiesMap(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	ts.Handler.c.adminUsersList.set("test|50|0", []byte("data"), time.Hour)

	ts.Handler.c.adminUsersList.clear()

	ts.Handler.c.adminUsersList.Lock()
	cnt := len(ts.Handler.c.adminUsersList.items)
	ts.Handler.c.adminUsersList.Unlock()
	if cnt != 0 {
		t.Errorf("items map not emptied, got %d entries", cnt)
	}
}

// ============================================================================
// /api/admin/promocodes cache (keyed by q)
// ============================================================================

func TestAdminPromoListCache_ClearEmptiesMap(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	ts.Handler.c.adminPromoList.set("test-q", []byte("data"), time.Hour)

	ts.Handler.c.adminPromoList.clear()

	ts.Handler.c.adminPromoList.Lock()
	cnt := len(ts.Handler.c.adminPromoList.items)
	ts.Handler.c.adminPromoList.Unlock()
	if cnt != 0 {
		t.Errorf("items map not emptied, got %d entries", cnt)
	}
}

// ============================================================================
// /api/admin/modes cache (keyed by q+limit+offset)
// ============================================================================

func TestAdminModesCache_ClearEmptiesMap(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	ts.Handler.c.adminModes.set("test|50|0", []byte("data"), time.Hour)

	ts.Handler.c.adminModes.clear()

	ts.Handler.c.adminModes.Lock()
	cnt := len(ts.Handler.c.adminModes.items)
	ts.Handler.c.adminModes.Unlock()
	if cnt != 0 {
		t.Errorf("items map not emptied, got %d entries", cnt)
	}
}

// ============================================================================
// adminMessagesTotal (60s cache for count(*) over dialogs_messages)
// ============================================================================

func TestAdminMessagesTotal_CachesAcrossCalls(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	// Clear the cache of this particular handler.
	ts.Handler.c.adminMessagesTotal.Lock()
	ts.Handler.c.adminMessagesTotal.expiresAt = time.Time{}
	ts.Handler.c.adminMessagesTotal.value = 0
	ts.Handler.c.adminMessagesTotal.Unlock()

	ctx := context.Background()
	v1 := ts.Handler.adminMessagesTotal(ctx)

	ts.Handler.c.adminMessagesTotal.Lock()
	cachedAt := ts.Handler.c.adminMessagesTotal.expiresAt
	cachedVal := ts.Handler.c.adminMessagesTotal.value
	ts.Handler.c.adminMessagesTotal.Unlock()

	if cachedAt.IsZero() {
		t.Errorf("expiresAt not set after call")
	}
	if cachedVal != v1 {
		t.Errorf("cached value mismatch: got %d, want %d", cachedVal, v1)
	}
	if cachedAt.Sub(time.Now()) > adminMessagesTotalTTL+5*time.Second {
		t.Errorf("TTL too large: %v", cachedAt.Sub(time.Now()))
	}

	// Second call must return the same value (even if the DB changed: the cache lives for its TTL).
	v2 := ts.Handler.adminMessagesTotal(ctx)
	if v1 != v2 {
		t.Errorf("repeated call returned different value: %d vs %d", v1, v2)
	}
}

func TestAdminMessagesTotal_ExpiryRefetches(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	ts.Handler.c.adminMessagesTotal.Lock()
	// Set expiry in the past.
	ts.Handler.c.adminMessagesTotal.expiresAt = time.Now().Add(-time.Minute)
	ts.Handler.c.adminMessagesTotal.value = -999 // impossible value
	ts.Handler.c.adminMessagesTotal.Unlock()

	ctx := context.Background()
	v := ts.Handler.adminMessagesTotal(ctx)
	if v == -999 {
		t.Errorf("expired cache returned stale value")
	}
	ts.Handler.c.adminMessagesTotal.Lock()
	expiresAt := ts.Handler.c.adminMessagesTotal.expiresAt
	ts.Handler.c.adminMessagesTotal.Unlock()
	if expiresAt.Before(time.Now()) {
		t.Errorf("expiresAt not refreshed after expiry: %v", expiresAt)
	}
}
