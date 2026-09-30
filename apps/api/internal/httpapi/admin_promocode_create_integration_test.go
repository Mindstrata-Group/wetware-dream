//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"mindstrata-stage1/api/internal/testsupport"
)

// ============================================================================
// PROMOCODE CREATE (POST /api/admin/promocodes)
// ============================================================================

// TestAdminPromoCreate_SingleCode: bulkCount=1 + explicit code + grants_type=mode.
func TestAdminPromoCreate_SingleCode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/promocodes", map[string]any{
		"code":         "TESTCODE_" + itoa(int64(env.Pool.Stat().AcquireCount())),
		"grantsType":   "mode",
		"targetIds":    []int64{mode.ID},
		"bulkCount":    1,
		"durationDays": 30,
		"maxUses":      5,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d body=%v", status, body)
	}
	created, _ := body["promocodes"].([]any)
	if len(created) != 1 {
		t.Fatalf("created count: got %d want 1", len(created))
	}
	row, _ := created[0].(map[string]any)
	if row["code"] == "" || row["id"] == nil {
		t.Errorf("created row missing fields: %v", row)
	}

	// Audit
	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action = 'admin.promocode.create'`).Scan(&auditCount)
	if auditCount == 0 {
		t.Errorf("admin.promocode.create audit not written")
	}
}

// TestAdminPromoCreate_BulkAutoGenerate: bulkCount=5 → 5 distinct codes with autoGenerate.
func TestAdminPromoCreate_BulkAutoGenerate(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/promocodes", map[string]any{
		"autoGenerate": true,
		"grantsType":   "mode",
		"targetIds":    []int64{mode.ID},
		"bulkCount":    5,
	})
	if status != http.StatusCreated {
		t.Fatalf("create bulk: %d body=%v", status, body)
	}
	created, _ := body["promocodes"].([]any)
	if len(created) != 5 {
		t.Errorf("created count: got %d want 5", len(created))
	}
	// All codes are unique.
	seen := map[string]bool{}
	for _, c := range created {
		row, _ := c.(map[string]any)
		code, _ := row["code"].(string)
		if seen[code] {
			t.Errorf("duplicate code in bulk: %s", code)
		}
		seen[code] = true
	}
}

// TestAdminPromoCreate_BulkOver200: bulkCount > 200 → 400/500.
func TestAdminPromoCreate_BulkOver200(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/promocodes", map[string]any{
		"autoGenerate": true,
		"grantsType":   "mode",
		"targetIds":    []int64{mode.ID},
		"bulkCount":    300,
	})
	if status == http.StatusCreated {
		t.Errorf("expected error for bulk > 200, got 201")
	}
}

// TestAdminPromoCreate_NoTargets: grants_type=mode without targetIds → error.
func TestAdminPromoCreate_NoTargets(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/promocodes", map[string]any{
		"grantsType": "mode",
		"targetIds":  []int64{},
		"bulkCount":  1,
	})
	if status == http.StatusCreated {
		t.Errorf("expected error without targets for grants_type=mode")
	}
}

// TestAdminPromoCreate_NonAdminBlocked.
func TestAdminPromoCreate_NonAdminBlocked(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "support"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/promocodes", map[string]any{
		"grantsType": "mode", "targetIds": []int64{1}, "bulkCount": 1,
	})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("support POST promocode: expected 403/401, got %d", status)
	}
}

// ============================================================================
// PROMOCODE DEACTIVATE / ACTIVATE / DELETE
// TestAdminPromoCreate_WithFirstModeId: pass firstModeId → it is saved in the DB.
func TestAdminPromoCreate_WithFirstModeId(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/promocodes", map[string]any{
		"grantsType":  "mode",
		"targetIds":   []int64{mode1.ID, mode2.ID},
		"firstModeId": mode2.ID,
		"bulkCount":   1,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d body=%v", status, body)
	}
	created, _ := body["promocodes"].([]any)
	if len(created) == 0 {
		t.Fatal("no promocodes returned")
	}
	row, _ := created[0].(map[string]any)
	id, _ := row["id"].(float64)
	if id == 0 {
		t.Fatalf("no id in response: %v", row)
	}

	var storedFirstModeID *int64
	err := env.Pool.QueryRow(context.Background(),
		`select first_mode_id from promocodes where id = $1`, int64(id)).Scan(&storedFirstModeID)
	if err != nil {
		t.Fatalf("query first_mode_id: %v", err)
	}
	if storedFirstModeID == nil || *storedFirstModeID != mode2.ID {
		t.Errorf("first_mode_id: got %v want %d", storedFirstModeID, mode2.ID)
	}
}

// TestApplyPromocode_FirstModeId_ReordersResponse: firstModeId → first element of modes[].
func TestApplyPromocode_FirstModeId_ReordersResponse(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	code := "FIRST_" + itoa(int64(env.Pool.Stat().AcquireCount()))

	promo := f.CreatePromocode(TestPromocodeOpts{Code: code, GrantsType: "mode", TargetID: mode1.ID})
	_, err := env.Pool.Exec(context.Background(),
		`insert into promocode_targets (promocode_id, target_id, created_at) values ($1, $2, now()), ($1, $3, now())`,
		promo.ID, mode1.ID, mode2.ID)
	if err != nil {
		t.Fatalf("insert promocode_targets: %v", err)
	}
	_, err = env.Pool.Exec(context.Background(),
		`update promocodes set first_mode_id = $2 where id = $1`, promo.ID, mode2.ID)
	if err != nil {
		t.Fatalf("set first_mode_id: %v", err)
	}

	ts.LoginAs(f.CreateSession(user.ID))
	status, body := httpJSON(t, ts, "POST", "/api/access/promocode/apply", map[string]any{"code": code})
	if status != http.StatusOK {
		t.Fatalf("apply: %d body=%v", status, body)
	}

	modes, _ := body["modes"].([]any)
	if len(modes) < 2 {
		t.Fatalf("expected >= 2 modes in response, got %d", len(modes))
	}
	firstMode, _ := modes[0].(map[string]any)
	firstID, _ := firstMode["modeId"].(float64)
	if int64(firstID) != mode2.ID {
		t.Errorf("first mode in response: got %v want %d", firstID, mode2.ID)
	}
}

// ============================================================================

// TestAdminPromocodeDetail_Deactivate: DELETE → active_to=now() + audit.
func TestAdminPromocodeDetail_Deactivate(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "DELETE", "/api/admin/promocodes/"+itoa(promo.ID), nil)
	if status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}

	// DB: active_to set to recent timestamp (within last minute)
	var n int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocodes where id = $1 and active_to is not null and active_to <= now() + interval '5 seconds'`,
		promo.ID).Scan(&n)
	if n != 1 {
		t.Errorf("active_to not set on deactivate")
	}
	// Audit
	var auditN int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action='admin.promocode.deactivate' and target_id=$1`,
		promo.ID).Scan(&auditN)
	if auditN == 0 {
		t.Errorf("deactivate audit not written")
	}
}

// TestAdminPromocodeDetail_PatchExtends: PATCH activeTo + maxUses → extension.
func TestAdminPromocodeDetail_PatchExtends(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(admin.ID))

	// PATCH with active-until 365 days ahead + max_uses=100
	status, _ := adminPatch(t, ts, "/api/admin/promocodes/"+itoa(promo.ID), map[string]any{
		"activeTo": "2099-01-01T00:00:00Z",
		"maxUses":  100,
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	// DB: max_uses updated (at least 100) and active_to in the future
	var maxUses int64
	var inFuture bool
	_ = env.Pool.QueryRow(context.Background(),
		`select max_uses, active_to > now() from promocodes where id = $1`,
		promo.ID).Scan(&maxUses, &inFuture)
	if maxUses < 100 {
		t.Errorf("max_uses not raised: got %d want >= 100", maxUses)
	}
	if !inFuture {
		t.Errorf("active_to not in future after activate-patch")
	}

	// Audit
	var n int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action='admin.promocode.activate' and target_id=$1`,
		promo.ID).Scan(&n)
	if n == 0 {
		t.Errorf("activate audit not written")
	}
}

// TestAdminPromocodeDetail_PatchActiveToInPast_400.
func TestAdminPromocodeDetail_PatchActiveToInPast_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := adminPatch(t, ts, "/api/admin/promocodes/"+itoa(promo.ID), map[string]any{
		"activeTo": "2020-01-01T00:00:00Z",
		"maxUses":  10,
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 for past activeTo, got %d", status)
	}
}

// TestAdminPromocodeDetail_PatchTargetsNewOnly: the set changes only for future activations.
func TestAdminPromocodeDetail_PatchTargetsNewOnly(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	userBefore := f.CreateUser(TestUserOpts{})
	userAfter := f.CreateUser(TestUserOpts{})
	modeOld := f.CreateMode(TestModeOpts{})
	modeNew := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: modeOld.ID, MaxUses: 5})

	ts.LoginAs(f.CreateSession(userBefore.ID))
	applyPromoOK(t, ts, promo.Code)

	ts.LoginAs(f.CreateSession(admin.ID))
	status, body := adminPatch(t, ts, "/api/admin/promocodes/"+itoa(promo.ID), map[string]any{
		"activeTo":    "2099-01-01T00:00:00Z",
		"maxUses":     5,
		"grantsType":  "mode",
		"targetIds":   []int64{modeNew.ID},
		"firstModeId": modeNew.ID,
		"updateScope": "new_only",
	})
	if status != http.StatusOK {
		t.Fatalf("patch new_only: %d body=%v", status, body)
	}

	assertPromoAccessActive(t, env.Pool, userBefore.ID, modeOld.ID, promo.ID, true)
	assertPromoAccessActive(t, env.Pool, userBefore.ID, modeNew.ID, promo.ID, false)

	ts.LoginAs(f.CreateSession(userAfter.ID))
	applyPromoOK(t, ts, promo.Code)
	assertPromoAccessActive(t, env.Pool, userAfter.ID, modeOld.ID, promo.ID, false)
	assertPromoAccessActive(t, env.Pool, userAfter.ID, modeNew.ID, promo.ID, true)
}

// TestAdminPromocodeDetail_PatchTargetsAllActivations: the set is rebuilt for those who already activated.
func TestAdminPromocodeDetail_PatchTargetsAllActivations(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	user := f.CreateUser(TestUserOpts{})
	modeOld := f.CreateMode(TestModeOpts{})
	modeNew := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: modeOld.ID, MaxUses: 5})

	ts.LoginAs(f.CreateSession(user.ID))
	applyPromoOK(t, ts, promo.Code)

	ts.LoginAs(f.CreateSession(admin.ID))
	status, body := adminPatch(t, ts, "/api/admin/promocodes/"+itoa(promo.ID), map[string]any{
		"activeTo":    "2099-01-01T00:00:00Z",
		"maxUses":     5,
		"grantsType":  "mode",
		"targetIds":   []int64{modeNew.ID},
		"firstModeId": modeNew.ID,
		"updateScope": "all_activations",
	})
	if status != http.StatusOK {
		t.Fatalf("patch all_activations: %d body=%v", status, body)
	}
	if updated, _ := body["updatedActivations"].(float64); updated < 1 {
		t.Fatalf("updatedActivations: got %v body=%v", body["updatedActivations"], body)
	}
	if closed, _ := body["deactivatedAccesses"].(float64); closed < 1 {
		t.Fatalf("deactivatedAccesses: got %v body=%v", body["deactivatedAccesses"], body)
	}

	assertPromoAccessRowExists(t, env.Pool, user.ID, modeOld.ID, promo.ID)
	assertPromoAccessActive(t, env.Pool, user.ID, modeOld.ID, promo.ID, false)
	assertPromoAccessActive(t, env.Pool, user.ID, modeNew.ID, promo.ID, true)
}

func assertPromoAccessActive(t *testing.T, pool *pgxpool.Pool, userID, modeID, promoID int64, want bool) {
	t.Helper()
	var count int64
	if err := pool.QueryRow(context.Background(), `
		select count(*)
		from user_mode_access
		where user_id = $1
		  and mode_id = $2
		  and access_type = 'promocode'
		  and source_id = $3
		  and active_from <= now()
		  and active_to > now()`, userID, modeID, promoID).Scan(&count); err != nil {
		t.Fatalf("query promo access: %v", err)
	}
	if got := count > 0; got != want {
		t.Fatalf("active promo access user=%d mode=%d promo=%d: got %v want %v", userID, modeID, promoID, got, want)
	}
}

func assertPromoAccessRowExists(t *testing.T, pool *pgxpool.Pool, userID, modeID, promoID int64) {
	t.Helper()
	var count int64
	if err := pool.QueryRow(context.Background(), `
		select count(*)
		from user_mode_access
		where user_id = $1
		  and mode_id = $2
		  and access_type = 'promocode'
		  and source_id = $3`, userID, modeID, promoID).Scan(&count); err != nil {
		t.Fatalf("query promo access row: %v", err)
	}
	if count == 0 {
		t.Fatalf("promo access row is missing user=%d mode=%d promo=%d", userID, modeID, promoID)
	}
}

// ============================================================================
// BULK DEACTIVATE
// ============================================================================

// TestAdminPromocodesBulkDeactivate_NoFilters: no dates → deactivates ALL active ones.
func TestAdminPromocodesBulkDeactivate_NoFilters(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode := f.CreateMode(TestModeOpts{})
	_ = f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	_ = f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	_ = f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	ts.LoginAs(f.CreateSession(admin.ID))

	// First count how many are active
	var beforeActive int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocodes where active_to is null or active_to > now()`).Scan(&beforeActive)

	status, body := httpJSON(t, ts, "POST", "/api/admin/promocodes/bulk-deactivate", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("bulk-deactivate: %d body=%v", status, body)
	}

	var afterActive int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocodes where active_to is null or active_to > now()`).Scan(&afterActive)

	if afterActive >= beforeActive {
		t.Errorf("bulk-deactivate didn't reduce active count: before=%d after=%d",
			beforeActive, afterActive)
	}
}

// TestAdminPromocodesBulkDeactivate_NonAdmin_403.
func TestAdminPromocodesBulkDeactivate_NonAdmin_403(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(user.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/promocodes/bulk-deactivate", map[string]any{})
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Errorf("billing_admin bulk-deactivate: expected 403/401, got %d", status)
	}
}
