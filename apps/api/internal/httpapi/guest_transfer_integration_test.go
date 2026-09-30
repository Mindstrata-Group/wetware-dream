//go:build integration

package httpapi

import (
	"context"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestGuestTransfer_AccessRowsCopied: the guest has user_mode_access →
// transferGuestAccess(guest, target) → target gets the same access, and the
// guest's active rows are removed (move, not copy; otherwise replaying the guest
// cookie clones paid access into new accounts, see the security review).
func TestGuestTransfer_AccessRowsCopied(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	guest := f.CreateUser(TestUserOpts{})
	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	src := int64(101)
	f.GrantAccess(GrantAccessOpts{
		UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 75,
		AccessType: "promocode", SourceID: &src,
	})

	// Before: target has no access
	var before int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1`, target.ID).Scan(&before)
	if before != 0 {
		t.Fatalf("target shouldn't have access before transfer (got %d rows)", before)
	}

	if err := h.transferGuestAccess(context.Background(), guest.ID, target.ID); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// After: target has the same row (limit=75, source_id=101)
	var copiedLimit int64
	var copiedSourceID int64
	err := env.Pool.QueryRow(context.Background(),
		`select daily_message_limit, source_id from user_mode_access
		 where user_id = $1 and mode_id = $2`,
		target.ID, mode.ID).Scan(&copiedLimit, &copiedSourceID)
	if err != nil {
		t.Fatalf("query target access: %v", err)
	}
	if copiedLimit != 75 {
		t.Fatalf("daily_message_limit copied: got %d want 75", copiedLimit)
	}
	if copiedSourceID != src {
		t.Fatalf("source_id: got %d want %d", copiedSourceID, src)
	}

	// The guest's active access is removed (move, not copy): replaying the guest
	// cookie can no longer clone the same access into another account.
	var guestAfter int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1`, guest.ID).Scan(&guestAfter)
	if guestAfter != 0 {
		t.Fatalf("guest's access row was not removed after transfer (got %d, want 0 — replay would clone access)", guestAfter)
	}
}

func TestGuestTransfer_UsedAccessReferencesMoved(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}
	ctx := context.Background()

	guest := f.CreateUser(TestUserOpts{})
	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	src := int64(303)
	f.GrantAccess(GrantAccessOpts{
		UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 75,
		AccessType: "promocode", SourceID: &src,
	})
	dialog := f.CreateDialog(guest.ID, mode.ID)
	messageID := f.AppendMessage(dialog.ID, "user", "guest message before oauth")

	var guestAccessID int64
	if err := env.Pool.QueryRow(ctx, `
		select id
		from user_mode_access
		where user_id = $1 and mode_id = $2 and access_type = 'promocode' and source_id = $3`,
		guest.ID, mode.ID, src).Scan(&guestAccessID); err != nil {
		t.Fatalf("query guest access: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into daily_mode_usage
			(user_id, mode_id, access_id, usage_date, messages_used, text_messages_used, audio_messages_used, created_at, updated_at)
		values ($1, $2, $3, current_date, 2, 2, 0, now(), now())`,
		guest.ID, mode.ID, guestAccessID); err != nil {
		t.Fatalf("insert guest daily usage: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into dialog_message_access_usage
			(user_id, mode_id, dialog_message_id, access_id, usage_date, usage_kind, created_at)
		values ($1, $2, $3, $4, current_date, 'message', now())`,
		guest.ID, mode.ID, messageID, guestAccessID); err != nil {
		t.Fatalf("insert guest message access usage: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into message_usage
			(user_id, mode_id, dialog_message_id, input_tokens, output_tokens, total_tokens, estimated_cost, access_id, is_compressed_response, created_at)
		values ($1, $2, $3, 10, 5, 15, 0.01, $4, false, now())`,
		guest.ID, mode.ID, messageID, guestAccessID); err != nil {
		t.Fatalf("insert guest message usage: %v", err)
	}

	if err := h.transferGuestAccess(ctx, guest.ID, target.ID); err != nil {
		t.Fatalf("transfer used access: %v", err)
	}

	var targetAccessID int64
	if err := env.Pool.QueryRow(ctx, `
		select id
		from user_mode_access
		where user_id = $1 and mode_id = $2 and access_type = 'promocode' and source_id = $3`,
		target.ID, mode.ID, src).Scan(&targetAccessID); err != nil {
		t.Fatalf("query target access: %v", err)
	}
	if targetAccessID == guestAccessID {
		t.Fatalf("target access reused guest id: %d", targetAccessID)
	}

	var dailyAccessID, dailyMessages int64
	if err := env.Pool.QueryRow(ctx, `
		select access_id, messages_used
		from daily_mode_usage
		where user_id = $1 and mode_id = $2 and usage_date = current_date`,
		target.ID, mode.ID).Scan(&dailyAccessID, &dailyMessages); err != nil {
		t.Fatalf("query target daily usage: %v", err)
	}
	if dailyAccessID != targetAccessID || dailyMessages != 2 {
		t.Fatalf("daily usage transfer: access=%d messages=%d want access=%d messages=2", dailyAccessID, dailyMessages, targetAccessID)
	}

	var dialogAccessID, messageAccessID int64
	if err := env.Pool.QueryRow(ctx, `
		select access_id
		from dialog_message_access_usage
		where user_id = $1 and dialog_message_id = $2`,
		target.ID, messageID).Scan(&dialogAccessID); err != nil {
		t.Fatalf("query target dialog access usage: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		select access_id
		from message_usage
		where user_id = $1 and dialog_message_id = $2`,
		target.ID, messageID).Scan(&messageAccessID); err != nil {
		t.Fatalf("query target message usage: %v", err)
	}
	if dialogAccessID != targetAccessID || messageAccessID != targetAccessID {
		t.Fatalf("usage access ids: dialog=%d message=%d want %d", dialogAccessID, messageAccessID, targetAccessID)
	}

	var guestRows int64
	if err := env.Pool.QueryRow(ctx, `
		select
			(select count(*) from user_mode_access where user_id = $1) +
			(select count(*) from daily_mode_usage where user_id = $1) +
			(select count(*) from dialog_message_access_usage where user_id = $1) +
			(select count(*) from message_usage where user_id = $1)`,
		guest.ID).Scan(&guestRows); err != nil {
		t.Fatalf("query guest rows: %v", err)
	}
	if guestRows != 0 {
		t.Fatalf("guest usage/access rows remained after transfer: %d", guestRows)
	}
}

// TestGuestTransfer_NoReplayToSecondTarget: after the guest's access moved to
// target1, a repeated transferGuestAccess(guest, target2) with the same guest
// cookie must NOT clone the same paid access into a second account.
func TestGuestTransfer_NoReplayToSecondTarget(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	guest := f.CreateUser(TestUserOpts{})
	target1 := f.CreateUser(TestUserOpts{})
	target2 := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	src := int64(202)
	f.GrantAccess(GrantAccessOpts{
		UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 75,
		AccessType: "promocode", SourceID: &src,
	})

	if err := h.transferGuestAccess(context.Background(), guest.ID, target1.ID); err != nil {
		t.Fatalf("first transfer: %v", err)
	}
	// Replay the same guest cookie on registering a second account.
	if err := h.transferGuestAccess(context.Background(), guest.ID, target2.ID); err != nil {
		t.Fatalf("second transfer (replay): %v", err)
	}

	var target2Count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1`, target2.ID).Scan(&target2Count)
	if target2Count != 0 {
		t.Fatalf("replayed guest cookie cloned access into second account (count=%d)", target2Count)
	}
}

// TestGuestTransfer_ExpiredAccessSkipped: expired access is not copied.
func TestGuestTransfer_ExpiredAccessSkipped(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	guest := f.CreateUser(TestUserOpts{})
	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})

	// Expired access on the guest
	past := timeAgo(48)
	pastPast := timeAgo(72)
	f.GrantAccess(GrantAccessOpts{
		UserID: guest.ID, ModeID: mode.ID, DailyMessageLimit: 50,
		ActiveFrom: &pastPast, ActiveTo: &past,
	})

	if err := h.transferGuestAccess(context.Background(), guest.ID, target.ID); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// Target must not get the expired access
	var targetCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1`, target.ID).Scan(&targetCount)
	if targetCount != 0 {
		t.Fatalf("expired access leaked to target (count=%d)", targetCount)
	}
}

// TestGuestTransfer_NoSelfTransfer: guest==target is a no-op without error.
func TestGuestTransfer_NoSelfTransfer(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})

	// guest==target → no-op
	if err := h.transferGuestAccess(context.Background(), user.ID, user.ID); err != nil {
		t.Fatalf("self-transfer should be no-op, got error: %v", err)
	}

	// access count = 1 (not duplicated)
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from user_mode_access where user_id = $1`, user.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("self-transfer duplicated rows: got %d want 1", count)
	}
}

// TestGuestTransfer_PromoUsagesCopied: promocode_usages are transferred (with duplicate protection).
func TestGuestTransfer_PromoUsagesCopied(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	h := Handler{DB: env.Pool}

	guest := f.CreateUser(TestUserOpts{})
	target := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{TargetID: mode.ID})

	// The guest applied a promo code (usage inserted directly, without the full flow)
	_, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
		guest.ID, promo.ID)
	if err != nil {
		t.Fatalf("insert guest usage: %v", err)
	}

	if err := h.transferGuestAccess(context.Background(), guest.ID, target.ID); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// Target has a usage of this promo code
	var targetUsages int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where user_id = $1 and promocode_id = $2`,
		target.ID, promo.ID).Scan(&targetUsages)
	if targetUsages != 1 {
		t.Fatalf("promo usage not transferred (target count=%d)", targetUsages)
	}

	// A repeated transfer does not duplicate (NOT EXISTS guard)
	if err := h.transferGuestAccess(context.Background(), guest.ID, target.ID); err != nil {
		t.Fatalf("second transfer: %v", err)
	}
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from promocode_usages where user_id = $1 and promocode_id = $2`,
		target.ID, promo.ID).Scan(&targetUsages)
	if targetUsages != 1 {
		t.Fatalf("second transfer duplicated promo usage: count=%d", targetUsages)
	}
}
