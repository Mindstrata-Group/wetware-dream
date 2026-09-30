package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func (h Handler) profileBillingSummary(ctx context.Context, userID int64) (map[string]any, error) {
	subscription := map[string]any{}
	err := h.DB.QueryRow(ctx, `
		select
		  s.id,
		  s.status,
		  s.active_to,
		  coalesce(t.name, ''),
		  coalesce(s.is_recurring, false),
		  coalesce(a.is_enabled, false),
		  a.next_retry_at,
		  a.grace_until,
		  coalesce(a.consecutive_failures, 0),
		  coalesce(a.cancel_reason, ''),
		  coalesce(a.last_error, ''),
		  case when pm.status = 'active' then pm.yookassa_payment_method_id else null end,
		  coalesce(pm.payment_method_type, ''),
		  coalesce(pm.status, '')
		from subscriptions s
		left join tariffs t on t.id = s.tariff_id
		left join autopay_subscriptions a on a.subscription_id = s.id
		left join payment_methods pm on pm.id = a.payment_method_id
		where s.user_id = $1
		order by s.active_to desc, s.id desc
		limit 1`, userID).Scan(
		scanMapValue(subscription, "id"),
		scanMapValue(subscription, "status"),
		scanMapValue(subscription, "activeTo"),
		scanMapValue(subscription, "tariffName"),
		scanMapValue(subscription, "isRecurring"),
		scanMapValue(subscription, "autoRenewEnabled"),
		scanMapValue(subscription, "nextRetryAt"),
		scanMapValue(subscription, "graceUntil"),
		scanMapValue(subscription, "consecutiveFailures"),
		scanMapValue(subscription, "cancelReason"),
		scanMapValue(subscription, "lastError"),
		scanMapValue(subscription, "paymentMethodId"),
		scanMapValue(subscription, "paymentMethodType"),
		scanMapValue(subscription, "paymentMethodStatus"),
	)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	payments := []map[string]any{}
	rows, err := h.DB.Query(ctx, `
		select
		  i.id,
		  i.yookassa_payment_id,
		  i.status,
		  coalesce(t.name, ''),
		  i.amount::text,
		  i.currency,
		  coalesce(i.subscription_months, 1),
		  coalesce(i.is_recurring, false),
		  coalesce(i.autorenew_requested, false),
		  coalesce(i.renewal_attempt, 0),
		  i.created_at,
		  i.paid_at,
		  coalesce(i.cancellation_reason, '')
		from invoices i
		left join tariffs t on t.id = i.tariff_id
		where i.user_id = $1
		order by i.created_at desc
		limit 20`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		item := map[string]any{}
		if err := rows.Scan(
			scanMapValue(item, "id"),
			scanMapValue(item, "paymentId"),
			scanMapValue(item, "status"),
			scanMapValue(item, "tariffName"),
			scanMapValue(item, "amount"),
			scanMapValue(item, "currency"),
			scanMapValue(item, "subscriptionMonths"),
			scanMapValue(item, "isRecurring"),
			scanMapValue(item, "autoRenewRequested"),
			scanMapValue(item, "renewalAttempt"),
			scanMapValue(item, "createdAt"),
			scanMapValue(item, "paidAt"),
			scanMapValue(item, "cancellationReason"),
		); err != nil {
			return nil, err
		}
		payments = append(payments, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"subscription": subscription, "payments": payments}, nil
}

func (h Handler) AdminPayments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if _, ok := h.requireAdminSection(w, r, "billing", false); !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		select
		  i.id,
		  i.user_id,
		  coalesce(u.email, ''),
		  coalesce(t.name, ''),
		  i.yookassa_payment_id,
		  i.status,
		  i.amount::text,
		  i.currency,
		  coalesce(i.is_recurring, false),
		  coalesce(i.autorenew_requested, false),
		  coalesce(i.renewal_attempt, 0),
		  i.created_at,
		  i.paid_at,
		  coalesce(i.cancellation_reason, ''),
		  s.id,
		  coalesce(a.is_enabled, false),
		  coalesce(s.status, ''),
		  a.next_retry_at,
		  a.grace_until,
		  coalesce(a.consecutive_failures, 0),
		  coalesce(pm.status, ''),
		  coalesce(pm.payment_method_type, '')
		from invoices i
		left join users u on u.id = i.user_id
		left join tariffs t on t.id = i.tariff_id
		left join recurring_payment_attempts rpa on rpa.yookassa_payment_id = i.yookassa_payment_id
		left join subscriptions s on s.id = (
		  select s2.id
		  from subscriptions s2
		  where s2.id = (
		    select a2.subscription_id
		    from autopay_subscriptions a2
		    where a2.id = rpa.autopay_subscription_id
		    limit 1
		  )
		     or s2.payment_reference = i.yookassa_payment_id
		  order by case when s2.payment_reference = i.yookassa_payment_id then 0 else 1 end, s2.id desc
		  limit 1
		)
		left join autopay_subscriptions a on a.subscription_id = s.id
		left join payment_methods pm on pm.id = a.payment_method_id
		order by i.created_at desc
		limit 100`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		item := map[string]any{}
		if err := rows.Scan(
			scanMapValue(item, "id"),
			scanMapValue(item, "userId"),
			scanMapValue(item, "email"),
			scanMapValue(item, "tariffName"),
			scanMapValue(item, "paymentId"),
			scanMapValue(item, "status"),
			scanMapValue(item, "amount"),
			scanMapValue(item, "currency"),
			scanMapValue(item, "isRecurring"),
			scanMapValue(item, "autoRenewRequested"),
			scanMapValue(item, "renewalAttempt"),
			scanMapValue(item, "createdAt"),
			scanMapValue(item, "paidAt"),
			scanMapValue(item, "cancellationReason"),
			scanMapValue(item, "subscriptionId"),
			scanMapValue(item, "autoRenewEnabled"),
			scanMapValue(item, "subscriptionStatus"),
			scanMapValue(item, "nextRetryAt"),
			scanMapValue(item, "graceUntil"),
			scanMapValue(item, "consecutiveFailures"),
			scanMapValue(item, "paymentMethodStatus"),
			scanMapValue(item, "paymentMethodType"),
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	runs, err := h.latestBillingRuns(r.Context(), 20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "payments": items, "billingRuns": runs})
}

func (h Handler) latestBillingRuns(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := h.DB.Query(ctx, `
		select id, run_key, run_type, source, dry_run, status, totals, error,
		       started_at, finished_at
		from billing_runs
		order by started_at desc, id desc
		limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		item := map[string]any{}
		var totalsBytes []byte
		if err := rows.Scan(
			scanMapValue(item, "id"),
			scanMapValue(item, "runKey"),
			scanMapValue(item, "runType"),
			scanMapValue(item, "source"),
			scanMapValue(item, "dryRun"),
			scanMapValue(item, "status"),
			&totalsBytes,
			scanMapValue(item, "error"),
			scanMapValue(item, "startedAt"),
			scanMapValue(item, "finishedAt"),
		); err != nil {
			return nil, err
		}
		var totals map[string]any
		if len(totalsBytes) > 0 {
			_ = json.Unmarshal(totalsBytes, &totals)
		}
		if totals == nil {
			totals = map[string]any{}
		}
		item["totals"] = totals
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanMapValue(target map[string]any, key string) any {
	return &mapScanner{target: target, key: key}
}

type mapScanner struct {
	target map[string]any
	key    string
}

func (s *mapScanner) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		s.target[s.key] = v
	case nil:
		s.target[s.key] = nil
	default:
		s.target[s.key] = v
	}
	return nil
}
