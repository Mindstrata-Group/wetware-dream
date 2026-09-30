//go:build integration

package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"mindstrata-stage1/api/internal/testsupport"
)

// =============================================================================
// chat_message_helpers.go — DB-backed helpers
// =============================================================================

// 1. TestGetDialogModeByID_HappyPath: dialog + grant → returns mode + hasActiveAccess=true.
func TestGetDialogModeByID_HappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "DialogModeTarget"})
	dialog := f.CreateDialog(user.ID, mode.ID)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 100})

	got, hasAccess, err := getDialogModeByID(context.Background(), env.Pool, user.ID, dialog.ID)
	if err != nil {
		t.Fatalf("getDialogModeByID: %v", err)
	}
	if got.ID != mode.ID {
		t.Errorf("mode id: got %d want %d", got.ID, mode.ID)
	}
	if got.Name != "DialogModeTarget" {
		t.Errorf("mode name: %q", got.Name)
	}
	if !hasAccess {
		t.Errorf("expected hasActiveAccess=true")
	}
	if got.OrchestratorCheckInterval <= 0 {
		t.Errorf("OrchestratorCheckInterval defaulted incorrectly: %d", got.OrchestratorCheckInterval)
	}
}

// 2. TestGetDialogModeByID_WrongUser_ErrNoRows: another user's userID → pgx.ErrNoRows.
// (Security invariant: one user cannot read another user's dialog.)
func TestGetDialogModeByID_WrongUser_ErrNoRows(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	owner := f.CreateUser(TestUserOpts{})
	intruder := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(owner.ID, mode.ID)

	_, _, err := getDialogModeByID(context.Background(), env.Pool, intruder.ID, dialog.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected pgx.ErrNoRows for wrong user, got %v", err)
	}
}

// 3. TestGetModeByDialogID_WithoutAccess_ErrNoRows: dialog + mode but no grant → ErrNoRows.
func TestGetModeByDialogID_WithoutAccess_ErrNoRows(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	// WITHOUT GrantAccess

	_, err := getModeByDialogID(context.Background(), env.Pool, user.ID, dialog.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected ErrNoRows when no active access, got %v", err)
	}
}

// 4. TestInsertDialogMessage_ReturnsIDAndTimestamp.
func TestInsertDialogMessage_ReturnsIDAndTimestamp(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)

	before := time.Now().Add(-2 * time.Second)
	msg, err := insertDialogMessage(context.Background(), env.Pool, dialog.ID, "user", "hello-from-test")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if msg.ID == 0 {
		t.Errorf("zero id")
	}
	if msg.Role != "user" {
		t.Errorf("role roundtrip: %q", msg.Role)
	}
	if msg.Content != "hello-from-test" {
		t.Errorf("content roundtrip: %q", msg.Content)
	}
	if msg.CreatedAt.Before(before) {
		t.Errorf("created_at older than pre-insert: %v vs %v", msg.CreatedAt, before)
	}

	// Verify in the DB.
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where id=$1 and dialog_id=$2`,
		msg.ID, dialog.ID).Scan(&count)
	if count != 1 {
		t.Errorf("inserted row not visible: count=%d", count)
	}
}

// 5. TestGetDialogMessages_ChronologicalOrder: returns in ascending created_at order
// (internally the function does SELECT DESC + reverse → ASC overall).
func TestGetDialogMessages_ChronologicalOrder(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)

	for i, c := range []string{"first", "second", "third", "fourth", "fifth"} {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		_ = f.AppendMessage(dialog.ID, role, c)
	}

	msgs, err := getDialogMessages(context.Background(), env.Pool, dialog.ID, 10, 0)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("got %d msgs want 5", len(msgs))
	}
	// ASC by time → first … fifth
	want := []string{"first", "second", "third", "fourth", "fifth"}
	for i, m := range msgs {
		if m.Content != want[i] {
			t.Errorf("position %d: got %q want %q", i, m.Content, want[i])
		}
	}
}

// 6. TestGetDialogMessages_BeforeIDFiltersOlder.
func TestGetDialogMessages_BeforeIDFiltersOlder(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)

	ids := []int64{}
	for _, c := range []string{"a", "b", "c", "d", "e"} {
		ids = append(ids, f.AppendMessage(dialog.ID, "user", c))
	}

	// beforeID = ids[3] → only messages with id < ids[3] must come back, i.e. a, b, c
	msgs, err := getDialogMessages(context.Background(), env.Pool, dialog.ID, 10, ids[3])
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("beforeID: got %d msgs want 3", len(msgs))
	}
	// limit=2 → must return only 2 (the last two by DESC = c,b → reversed = b,c)
	limited, err := getDialogMessages(context.Background(), env.Pool, dialog.ID, 2, ids[3])
	if err != nil {
		t.Fatalf("limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("limit=2: got %d want 2", len(limited))
	}
	// the last one must be 'c' (closest to beforeID, last in ASC)
	if limited[len(limited)-1].Content != "c" {
		t.Errorf("expected last msg 'c', got %q", limited[len(limited)-1].Content)
	}
}

// =============================================================================
// admin_export_promo_modes.go — DB-backed helpers
// =============================================================================

// 7. TestAdminExportGroupedModeIDs_DedupesByPair: a duplicate (group, mode) → once.
func TestAdminExportGroupedModeIDs_DedupesByPair(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{})

	// Imitate "two rows of the same (group, mode)" with an inline VALUES query.
	sql := `select v.group_id, v.mode_id from (values
		(7::bigint, ` + itoa(mode.ID) + `::bigint),
		(7::bigint, ` + itoa(mode.ID) + `::bigint),
		(8::bigint, ` + itoa(mode.ID) + `::bigint)
	) as v(group_id, mode_id)`

	got, err := h.adminExportGroupedModeIDs(context.Background(), sql)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got[7]) != 1 {
		t.Errorf("group 7: dedup failed, got %v", got[7])
	}
	if got[7][0] != mode.ID {
		t.Errorf("group 7 mode: got %d want %d", got[7][0], mode.ID)
	}
	if len(got[8]) != 1 {
		t.Errorf("group 8: got %v", got[8])
	}
}

// 8. TestPromocodeTargetIDs_TargetsTablePreferred: rows exist in promocode_targets → legacy is ignored.
func TestPromocodeTargetIDs_TargetsTablePreferred(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode1 := f.CreateMode(TestModeOpts{})
	mode2 := f.CreateMode(TestModeOpts{})
	legacy := f.CreateMode(TestModeOpts{}) // must not end up in the result
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: legacy.ID})

	// Fill promocode_targets:
	_, err := env.Pool.Exec(context.Background(),
		`insert into promocode_targets (promocode_id, target_id) values ($1, $2), ($1, $3)`,
		promo.ID, mode1.ID, mode2.ID)
	if err != nil {
		t.Fatalf("seed targets: %v", err)
	}

	got, err := h.promocodeTargetIDs(context.Background(), promo.ID, legacy.ID)
	if err != nil {
		t.Fatalf("targetIDs: %v", err)
	}
	if !containsInt64(got, mode1.ID) || !containsInt64(got, mode2.ID) {
		t.Errorf("missing expected target IDs: got %v want [%d,%d]", got, mode1.ID, mode2.ID)
	}
	if containsInt64(got, legacy.ID) {
		t.Errorf("legacy target leaked when targets table populated: %v", got)
	}
}

// 9. TestPromocodeTargetIDs_FallsBackToLegacy: empty targets, legacy > 0 → [legacyID].
func TestPromocodeTargetIDs_FallsBackToLegacy(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})
	// promocode_targets is empty for this promo.ID

	got, err := h.promocodeTargetIDs(context.Background(), promo.ID, mode.ID)
	if err != nil {
		t.Fatalf("targetIDs: %v", err)
	}
	if len(got) != 1 || got[0] != mode.ID {
		t.Errorf("legacy fallback: got %v want [%d]", got, mode.ID)
	}

	// If legacy = 0 → empty result.
	got2, err := h.promocodeTargetIDs(context.Background(), promo.ID, 0)
	if err != nil {
		t.Fatalf("targetIDs no-legacy: %v", err)
	}
	if len(got2) != 0 {
		t.Errorf("empty targets + legacy=0: got %v want []", got2)
	}
}

// 10. TestPromocodeModeIDsForTargets_TariffAndGroupExpansion.
func TestPromocodeModeIDsForTargets_TariffAndGroupExpansion(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	modeA := f.CreateMode(TestModeOpts{})
	modeB := f.CreateMode(TestModeOpts{})

	// Create a tariff_group + a tariff in it + tariff_mode for two modes.
	var groupID int64
	if err := env.Pool.QueryRow(context.Background(),
		`insert into tariff_groups (name, sort_order) values ('PromoExpansionGroup', 0) returning id`).Scan(&groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	var tariffID int64
	if err := env.Pool.QueryRow(context.Background(),
		`insert into tariffs (name, group_id, monthly_price, limit_type) values ('PromoExpansionTariff', $1, 0, 'shared') returning id`, groupID).Scan(&tariffID); err != nil {
		t.Fatalf("seed tariff: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(),
		`insert into tariff_mode (tariff_id, mode_id) values ($1, $2), ($1, $3)`,
		tariffID, modeA.ID, modeB.ID); err != nil {
		t.Fatalf("seed tariff_mode: %v", err)
	}

	// grants_type=tariff with target=tariffID → expanded to [modeA, modeB]
	got, err := h.promocodeModeIDsForTargets(context.Background(), "tariff", []int64{tariffID})
	if err != nil {
		t.Fatalf("tariff expansion: %v", err)
	}
	if !containsInt64(got, modeA.ID) || !containsInt64(got, modeB.ID) {
		t.Errorf("tariff expansion missing modes: got %v want both %d,%d", got, modeA.ID, modeB.ID)
	}

	// grants_type=tariff_group with target=groupID → expanded via tariffs.group_id + tariff_mode.
	got2, err := h.promocodeModeIDsForTargets(context.Background(), "tariff_group", []int64{groupID})
	if err != nil {
		t.Fatalf("group expansion: %v", err)
	}
	if !containsInt64(got2, modeA.ID) || !containsInt64(got2, modeB.ID) {
		t.Errorf("group expansion missing modes: got %v", got2)
	}

	// Empty targetIDs for tariff → empty result (early exit).
	gotEmpty, err := h.promocodeModeIDsForTargets(context.Background(), "tariff", []int64{})
	if err != nil {
		t.Fatalf("empty targets tariff: %v", err)
	}
	if len(gotEmpty) != 0 {
		t.Errorf("empty targets should give empty result: got %v", gotEmpty)
	}

	// Unknown grants_type → empty result.
	gotUnknown, err := h.promocodeModeIDsForTargets(context.Background(), "unknown_thing", []int64{1})
	if err != nil {
		t.Fatalf("unknown type: %v", err)
	}
	if len(gotUnknown) != 0 {
		t.Errorf("unknown grants_type: got %v want []", gotUnknown)
	}
}

// helper
func containsInt64(s []int64, v int64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
