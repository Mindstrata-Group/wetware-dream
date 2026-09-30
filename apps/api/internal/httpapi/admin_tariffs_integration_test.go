//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// ============================================================================
// TARIFFS CRUD (POST /api/admin/tariffs + GET/PATCH/DELETE /api/admin/tariffs/{id})
// ============================================================================

// TestTariffCRUD_CreateMinimal: minimal valid POST → 201 + DB row + audit.
func TestTariffCRUD_CreateMinimal(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name":         "Test Tariff",
		"description":  "for tests",
		"monthlyPrice": 990.0,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d body=%v", status, body)
	}
	tid, _ := body["tariffId"].(float64)
	if tid == 0 {
		t.Fatalf("no tariffId in response: %v", body)
	}

	// DB: row created
	var name, ttype, ltype string
	var price float64
	err := env.Pool.QueryRow(context.Background(),
		`select name, tariff_type, limit_type, monthly_price from tariffs where id = $1`,
		int64(tid)).Scan(&name, &ttype, &ltype, &price)
	if err != nil {
		t.Fatalf("query tariff: %v", err)
	}
	if name != "Test Tariff" {
		t.Errorf("name: got %q", name)
	}
	// Defaults: tariff_type=regular, limit_type=shared.
	if ttype != "regular" || ltype != "shared" {
		t.Errorf("defaults: type=%q limit_type=%q (want regular/shared)", ttype, ltype)
	}

	// Audit
	var auditCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action = 'admin.tariff.create' and target_id = $1`,
		int64(tid)).Scan(&auditCount)
	if auditCount == 0 {
		t.Errorf("admin.tariff.create audit not written")
	}
}

// TestTariffCRUD_CreateWithModes: POST with modeIds → tariff_mode filled, firstModeId saved.
func TestTariffCRUD_CreateWithModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name":        "Premium",
		"modeIds":     []int64{mode1.ID, mode2.ID},
		"firstModeId": mode2.ID,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d body=%v", status, body)
	}
	tid, _ := body["tariffId"].(float64)

	// DB: tariff_mode has both rows
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from tariff_mode where tariff_id = $1`, int64(tid)).Scan(&count)
	if count != 2 {
		t.Errorf("tariff_mode count: got %d want 2", count)
	}
	var firstModeID int64
	_ = env.Pool.QueryRow(context.Background(),
		`select first_mode_id from tariffs where id = $1`, int64(tid)).Scan(&firstModeID)
	if firstModeID != mode2.ID {
		t.Errorf("first_mode_id: got %d want %d", firstModeID, mode2.ID)
	}
}

// TestTariffCRUD_CreateRejectsFirstModeOutsideModes: the lead mode must be part of the tariff's mode set.
func TestTariffCRUD_CreateRejectsFirstModeOutsideModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name":        "Bad first mode",
		"modeIds":     []int64{mode1.ID},
		"firstModeId": mode2.ID,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid firstModeId, got %d body=%v", status, body)
	}
}

// TestTariffCRUD_CreateInvalidType: tariffType=garbage → 400.
func TestTariffCRUD_CreateInvalidType(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name":       "Bad",
		"tariffType": "invalid",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid tariffType, got %d", status)
	}
}

// TestTariffCRUD_CreateMissingName: empty name → 400.
func TestTariffCRUD_CreateMissingName(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d", status)
	}
}

// TestTariffCRUD_GetByID: create → GET /api/admin/tariffs/{id} → details.
func TestTariffCRUD_GetByID(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name": "ForGet", "description": "desc",
	})
	tid, _ := body["tariffId"].(float64)

	status, getBody := httpJSON(t, ts, "GET", "/api/admin/tariffs/"+itoa(int64(tid)), nil)
	if status != http.StatusOK {
		t.Fatalf("get: %d body=%v", status, getBody)
	}
	tariff, _ := getBody["tariff"].(map[string]any)
	if tariff == nil {
		t.Fatalf("no tariff in response: %v", getBody)
	}
	if got, _ := tariff["name"].(string); got != "ForGet" {
		t.Errorf("name: got %q", got)
	}
}

// TestTariffCRUD_GetNotFound: GET for a missing one → 404.
func TestTariffCRUD_GetNotFound(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "GET", "/api/admin/tariffs/999999", nil)
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
}

// TestTariffCRUD_PatchUpdatesNameAndModes: PATCH changes name + modes (full replacement).
func TestTariffCRUD_PatchUpdatesNameAndModes(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	mode3 := f.CreateMode(TestModeOpts{})
	ts.LoginAs(f.CreateSession(admin.ID))

	// Create with modes 1,2
	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name": "ToPatch", "modeIds": []int64{mode1.ID, mode2.ID},
	})
	tid, _ := body["tariffId"].(float64)

	// PATCH: change name + only mode3 (the old 1,2 go away)
	status, _ := adminPatch(t, ts, "/api/admin/tariffs/"+itoa(int64(tid)), map[string]any{
		"name":                     "Patched",
		"modeIds":                  []int64{mode3.ID},
		"firstModeId":              mode3.ID,
		"availableForSubscription": true,
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}

	// DB: name updated, tariff_mode has only mode3
	var newName string
	_ = env.Pool.QueryRow(context.Background(),
		`select name from tariffs where id = $1`, int64(tid)).Scan(&newName)
	if newName != "Patched" {
		t.Errorf("name after patch: got %q", newName)
	}
	var modeID int64
	_ = env.Pool.QueryRow(context.Background(),
		`select mode_id from tariff_mode where tariff_id = $1`, int64(tid)).Scan(&modeID)
	if modeID != mode3.ID {
		t.Errorf("tariff_mode after patch: got mode %d want %d", modeID, mode3.ID)
	}
	var firstModeID int64
	_ = env.Pool.QueryRow(context.Background(),
		`select first_mode_id from tariffs where id = $1`, int64(tid)).Scan(&firstModeID)
	if firstModeID != mode3.ID {
		t.Errorf("first_mode_id after patch: got %d want %d", firstModeID, mode3.ID)
	}
}

// TestTariffCRUD_DeleteArchives: DELETE of an active one → archived_at + available=false.
func TestTariffCRUD_DeleteArchives(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name": "ToArchive", "availableForSubscription": true,
	})
	tid, _ := body["tariffId"].(float64)

	status, delBody := httpJSON(t, ts, "DELETE", "/api/admin/tariffs/"+itoa(int64(tid)), nil)
	if status != http.StatusOK {
		t.Fatalf("delete: %d body=%v", status, delBody)
	}
	// Should report archived=true
	if archived, _ := delBody["archived"].(bool); !archived {
		t.Errorf("expected archived=true, body=%v", delBody)
	}

	// DB: archived_at != null
	var archivedAt *string
	_ = env.Pool.QueryRow(context.Background(),
		`select archived_at::text from tariffs where id = $1`, int64(tid)).Scan(&archivedAt)
	if archivedAt == nil {
		t.Errorf("archived_at not set after first delete")
	}
}

// TestTariffCRUD_DeleteArchivedDestroys: a second DELETE on an archived one → really deletes.
func TestTariffCRUD_DeleteArchivedDestroys(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name": "ToDestroy", "availableForSubscription": false,
	})
	tid, _ := body["tariffId"].(float64)

	status, delBody := httpJSON(t, ts, "DELETE", "/api/admin/tariffs/"+itoa(int64(tid)), nil)
	if status != http.StatusOK {
		t.Fatalf("delete: %d body=%v", status, delBody)
	}
	if deleted, _ := delBody["deleted"].(bool); !deleted {
		t.Errorf("expected deleted=true for archived tariff, body=%v", delBody)
	}

	// DB: row gone
	var n int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from tariffs where id = $1`, int64(tid)).Scan(&n)
	if n != 0 {
		t.Errorf("tariff still exists after destroy: %d rows", n)
	}
}

// TestTariffCRUD_ListAll: GET /api/admin/tariffs.
func TestTariffCRUD_ListAll(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, _ = httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{"name": "T1"})
	_, _ = httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{"name": "T2"})

	status, body := httpJSON(t, ts, "GET", "/api/admin/tariffs", nil)
	if status != http.StatusOK {
		t.Fatalf("list: %d", status)
	}
	tariffs, _ := body["tariffs"].([]any)
	if len(tariffs) < 2 {
		t.Errorf("list count: got %d want >= 2", len(tariffs))
	}
}

// TestTariffCRUD_BillingAdminCanRead_ButNotMutate: billing_admin can GET, but not POST.
func TestTariffCRUD_BillingAdminCanRead_ButNotMutate(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	ba := f.CreateUser(TestUserOpts{Role: "billing_admin"})
	ts.LoginAs(f.CreateSession(ba.ID))

	// GET — ok
	status, _ := httpJSON(t, ts, "GET", "/api/admin/tariffs", nil)
	if status != http.StatusOK {
		t.Errorf("billing_admin GET /api/admin/tariffs: %d", status)
	}
	// POST — forbidden
	status, _ = httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{"name": "Naughty"})
	if status != http.StatusForbidden {
		t.Errorf("billing_admin POST: expected 403, got %d", status)
	}
}

// ============================================================================
// TARIFF GROUPS CRUD
// ============================================================================

// TestTariffGroup_CreateAndList.
func TestTariffGroup_CreateAndList(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := httpJSON(t, ts, "POST", "/api/admin/tariff-groups", map[string]any{
		"name":        "TestGroup",
		"description": "for tests",
	})
	if status != http.StatusCreated {
		t.Fatalf("create group: %d body=%v", status, body)
	}
	gid, _ := body["groupId"].(float64)
	if gid == 0 {
		t.Fatalf("no groupId returned")
	}

	// Audit
	var n int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from admin_audit_log where action = 'admin.tariff_group.create' and target_id = $1`,
		int64(gid)).Scan(&n)
	if n == 0 {
		t.Errorf("audit not written")
	}

	// List
	listStatus, listBody := httpJSON(t, ts, "GET", "/api/admin/tariff-groups", nil)
	if listStatus != http.StatusOK {
		t.Fatalf("list groups: %d", listStatus)
	}
	groups, _ := listBody["groups"].([]any)
	if len(groups) == 0 {
		t.Errorf("groups list empty after create")
	}
}

// TestTariffGroup_CreateEmptyName_400.
func TestTariffGroup_CreateEmptyName_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, _ := httpJSON(t, ts, "POST", "/api/admin/tariff-groups", map[string]any{
		"name": "",
	})
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
}

// TestTariffGroup_PatchAndDelete.
func TestTariffGroup_PatchAndDelete(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, body := httpJSON(t, ts, "POST", "/api/admin/tariff-groups", map[string]any{
		"name": "OriginalGroup",
	})
	gid, _ := body["groupId"].(float64)

	// PATCH
	status, _ := adminPatch(t, ts, "/api/admin/tariff-groups/"+itoa(int64(gid)), map[string]any{
		"name": "RenamedGroup",
	})
	if status != http.StatusOK {
		t.Fatalf("patch: %d", status)
	}
	var newName string
	_ = env.Pool.QueryRow(context.Background(),
		`select name from tariff_groups where id = $1`, int64(gid)).Scan(&newName)
	if newName != "RenamedGroup" {
		t.Errorf("name after patch: got %q", newName)
	}

	// DELETE
	delStatus, _ := httpJSON(t, ts, "DELETE", "/api/admin/tariff-groups/"+itoa(int64(gid)), nil)
	if delStatus != http.StatusOK {
		t.Fatalf("delete: %d", delStatus)
	}
	var n int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from tariff_groups where id = $1`, int64(gid)).Scan(&n)
	if n != 0 {
		t.Errorf("group still exists after delete")
	}
}

// TestTariffGroup_DeleteDetachesTariffs: the group's tariffs → group_id = null after the group is deleted.
func TestTariffGroup_DeleteDetachesTariffs(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	_, gBody := httpJSON(t, ts, "POST", "/api/admin/tariff-groups", map[string]any{
		"name": "WithTariffs",
	})
	gid, _ := gBody["groupId"].(float64)

	gidInt := int64(gid)
	_, tBody := httpJSON(t, ts, "POST", "/api/admin/tariffs", map[string]any{
		"name":    "InGroup",
		"groupId": gidInt,
	})
	tid, _ := tBody["tariffId"].(float64)

	// Delete group
	_, _ = httpJSON(t, ts, "DELETE", "/api/admin/tariff-groups/"+itoa(gidInt), nil)

	// Tariff still exists but group_id = null
	var groupID *int64
	_ = env.Pool.QueryRow(context.Background(),
		`select group_id from tariffs where id = $1`, int64(tid)).Scan(&groupID)
	if groupID != nil {
		t.Errorf("tariff.group_id not nulled after group deleted: got %v", *groupID)
	}
}
