//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestAdminAccessRecoveryTransfersUnusedPaidAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	sourceID, modeID, tariffID := seedRecurringTariff(t, env)
	target := f.CreateUser(TestUserOpts{Email: "recover-target@test.local"})
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ctx := context.Background()
	var invoiceID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, paid_at, is_recurring, autorenew_requested
		)
		values ($1, $2, 'yk-lost-cookie', 100.00, 'RUB', 'paid', 1, now() + interval '1 month', now(), true, true)
		returning id`, sourceID, tariffID).Scan(&invoiceID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into user_mode_access (user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id)
		values ($1, $2, now(), now() + interval '1 month', 50, 0, 'subscription', $3)`, sourceID, modeID, invoiceID); err != nil {
		t.Fatalf("seed access: %v", err)
	}
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, http.MethodGet, "/api/admin/payments/access-recovery?q=yk-lost-cookie", nil)
	if status != http.StatusOK {
		t.Fatalf("search status: got %d body=%v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("search items: got %d want 1 body=%v", len(items), body)
	}
	item, _ := items[0].(map[string]any)
	if item["transferableWithoutRisk"] != true {
		t.Fatalf("transferableWithoutRisk: got %v want true", item["transferableWithoutRisk"])
	}

	status, body = httpJSON(t, ts, http.MethodPost, "/api/admin/payments/access-recovery", map[string]any{
		"invoiceId":   invoiceID,
		"targetEmail": target.Email,
	})
	if status != http.StatusOK {
		t.Fatalf("transfer status: got %d body=%v", status, body)
	}
	var targetAccessActive, sourceAccessActive bool
	var invoiceUserID int64
	if err := env.Pool.QueryRow(ctx, `
		select
		  exists(select 1 from user_mode_access where user_id = $1 and mode_id = $3 and source_id = $4 and active_to > now()),
		  exists(select 1 from user_mode_access where user_id = $2 and mode_id = $3 and source_id = $4 and active_to > now()),
		  (select user_id from invoices where id = $4)`,
		target.ID, sourceID, modeID, invoiceID).Scan(&targetAccessActive, &sourceAccessActive, &invoiceUserID); err != nil {
		t.Fatalf("query transfer state: %v", err)
	}
	if !targetAccessActive || sourceAccessActive || invoiceUserID != target.ID {
		t.Fatalf("transfer state: targetActive=%v sourceActive=%v invoiceUser=%d want targetActive true/source false/invoice target",
			targetAccessActive, sourceAccessActive, invoiceUserID)
	}
}

func TestAdminAccessRecoveryRefusesUsedPaidAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	sourceID, modeID, tariffID := seedRecurringTariff(t, env)
	target := f.CreateUser(TestUserOpts{Email: "recover-used-target@test.local"})
	owner := f.CreateUser(TestUserOpts{Role: "owner"})
	ctx := context.Background()
	var invoiceID, accessID int64
	if err := env.Pool.QueryRow(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, paid_at, is_recurring, autorenew_requested
		)
		values ($1, $2, 'yk-used-cookie', 100.00, 'RUB', 'paid', 1, now() + interval '1 month', now(), true, true)
		returning id`, sourceID, tariffID).Scan(&invoiceID); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `
		insert into user_mode_access (user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id)
		values ($1, $2, now(), now() + interval '1 month', 50, 0, 'subscription', $3)
		returning id`, sourceID, modeID, invoiceID).Scan(&accessID); err != nil {
		t.Fatalf("seed access: %v", err)
	}
	if _, err := env.Pool.Exec(ctx, `
		insert into daily_mode_usage (user_id, mode_id, access_id, usage_date, messages_used, text_messages_used, audio_messages_used)
		values ($1, $2, $3, $4, 1, 1, 0)`, sourceID, modeID, accessID, time.Now()); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(owner.ID))

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/payments/access-recovery", map[string]any{
		"invoiceId":    invoiceID,
		"targetUserId": target.ID,
		"reason":       "lost_cookie_recovery",
	})
	if status != http.StatusConflict {
		t.Fatalf("used transfer status: got %d body=%v", status, body)
	}
}
