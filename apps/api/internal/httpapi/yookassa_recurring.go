package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const yookassaRenewalGrace = 3 * 24 * time.Hour

type yookassaCreatePaymentResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Confirmation *struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
}

type adminRunDueRenewalsRequest struct {
	DryRun         bool   `json:"dryRun"`
	RunKey         string `json:"runKey"`
	ReconcileLimit int    `json:"reconcileLimit"`
}

func (h Handler) createYooKassaPayment(ctx context.Context, payload map[string]any, idempotenceKey string) (yookassaCreatePaymentResult, error) {
	var result yookassaCreatePaymentResult
	creds, err := h.activeYooKassaCredentials(ctx)
	if err != nil {
		return result, err
	}
	if !creds.configured() {
		return result, fmt.Errorf("yookassa is not configured")
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.yookassa.ru/v3/payments", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotence-Key", idempotenceKey)
	httpReq.Header.Set("Authorization", creds.basicAuth())

	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return result, fmt.Errorf("decode yookassa response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("yookassa status %d", resp.StatusCode)
	}
	if strings.TrimSpace(result.ID) == "" {
		return result, fmt.Errorf("yookassa response missing payment id")
	}
	return result, nil
}

func (h Handler) AdminYooKassaRunDueRenewals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	admin, ok := h.requireRole(w, r, "owner", "admin", "billing_admin")
	if !ok {
		return
	}
	var req adminRunDueRenewalsRequest
	if r.Body != nil {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
	}
	result, err := h.runAdminYooKassaBilling(r.Context(), admin.ID, req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	result["ok"] = true
	writeJSON(w, http.StatusOK, result)
}

func (h Handler) runAdminYooKassaBilling(ctx context.Context, adminID int64, req adminRunDueRenewalsRequest) (map[string]any, error) {
	if req.ReconcileLimit <= 0 || req.ReconcileLimit > 100 {
		req.ReconcileLimit = 50
	}
	runKey := strings.TrimSpace(req.RunKey)
	if runKey == "" {
		runKey = fmt.Sprintf("admin-yookassa-%d-%d", adminID, time.Now().UTC().UnixNano())
	}
	runID, alreadySucceeded, existing, err := h.startBillingRun(ctx, runKey, "admin", req.DryRun)
	if err != nil {
		return nil, err
	}
	if alreadySucceeded {
		existing["alreadySucceeded"] = true
		return existing, nil
	}

	status := "succeeded"
	errorText := ""
	totals := map[string]any{}
	defer func() {
		_ = h.finishBillingRun(context.Background(), runID, status, totals, errorText)
	}()

	reconcile := map[string]int{}
	if req.DryRun {
		reconcile, err = h.countPendingYooKassaInvoices(ctx, req.ReconcileLimit)
		if err == nil {
			err = h.recordBillingRunItem(ctx, runID, "reconcile_pending_invoices", "skipped", reconcile, "dry_run")
		}
	} else {
		reconcile, err = h.ReconcilePendingYooKassaInvoices(ctx, req.ReconcileLimit)
		if err == nil {
			err = h.recordBillingRunItem(ctx, runID, "reconcile_pending_invoices", "succeeded", reconcile, "")
		}
	}
	if err != nil {
		status, errorText = "failed", err.Error()
		_ = h.recordBillingRunItem(ctx, runID, "reconcile_pending_invoices", "failed", reconcile, err.Error())
		return nil, err
	}
	totals["reconcile"] = reconcile

	processed := 0
	if req.DryRun {
		due, err := h.countDueYooKassaRenewals(ctx)
		if err != nil {
			status, errorText = "failed", err.Error()
			_ = h.recordBillingRunItem(ctx, runID, "run_due_renewals", "failed", map[string]int{"due": due}, err.Error())
			return nil, err
		}
		if err := h.recordBillingRunItem(ctx, runID, "run_due_renewals", "skipped", map[string]int{"due": due}, "dry_run"); err != nil {
			status, errorText = "failed", err.Error()
			return nil, err
		}
		totals["renewals"] = map[string]any{"processed": 0, "due": due}
	} else {
		processed, err = h.ProcessDueYooKassaRenewals(ctx, time.Now())
		if err != nil {
			status, errorText = "failed", err.Error()
			_ = h.recordBillingRunItem(ctx, runID, "run_due_renewals", "failed", map[string]int{"processed": processed}, err.Error())
			return nil, err
		}
		if err := h.recordBillingRunItem(ctx, runID, "run_due_renewals", "succeeded", map[string]int{"processed": processed}, ""); err != nil {
			status, errorText = "failed", err.Error()
			return nil, err
		}
		totals["renewals"] = map[string]any{"processed": processed}
	}

	exhausted, err := h.markExhaustedYooKassaRenewals(ctx, req.DryRun)
	if err != nil {
		status, errorText = "failed", err.Error()
		_ = h.recordBillingRunItem(ctx, runID, "mark_exhausted_renewals", "failed", exhausted, err.Error())
		return nil, err
	}
	stepStatus, stepError := "succeeded", ""
	if req.DryRun {
		stepStatus, stepError = "skipped", "dry_run"
	}
	if err := h.recordBillingRunItem(ctx, runID, "mark_exhausted_renewals", stepStatus, exhausted, stepError); err != nil {
		status, errorText = "failed", err.Error()
		return nil, err
	}
	totals["exhausted"] = exhausted

	return map[string]any{
		"runId":     runID,
		"runKey":    runKey,
		"dryRun":    req.DryRun,
		"processed": processed,
		"totals":    totals,
	}, nil
}

func (h Handler) DisableYooKassaAutoRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	disabled, methodsDisabled, err := h.disableUserAutoRenewAndPaymentMethods(r.Context(), user.ID, "user_disabled")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "disabled": disabled, "paymentMethodsDisabled": methodsDisabled})
}

func (h Handler) disableUserAutoRenewAndPaymentMethods(ctx context.Context, userID int64, reason string) (int64, int64, error) {
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	ct, err := tx.Exec(ctx, `
		update autopay_subscriptions a
		set is_enabled = false,
		    cancel_reason = $2,
		    updated_at = now()
		from subscriptions s
		where a.subscription_id = s.id
		  and s.user_id = $1
		  and a.is_enabled = true`, userID, reason)
	if err != nil {
		return 0, 0, err
	}
	disabled := ct.RowsAffected()

	ct, err = tx.Exec(ctx, `
		update payment_methods pm
		set status = 'disabled',
		    yookassa_payment_method_id = 'disabled:' || pm.id::text || ':' || extract(epoch from now())::bigint::text,
		    updated_at = now()
		where pm.status = 'active'
		  and exists (
		    select 1
		    from autopay_subscriptions a
		    join subscriptions s on s.id = a.subscription_id
		    where a.payment_method_id = pm.id
		      and s.user_id = $1
		      and a.is_enabled = false
		      and a.cancel_reason = $2
		  )`, userID, reason)
	if err != nil {
		return 0, 0, err
	}
	methodsDisabled := ct.RowsAffected()

	if _, err := tx.Exec(ctx, `
		update subscriptions
		set is_recurring = false,
		    autopay_disabled_reason = $2,
		    updated_at = now()
		where user_id = $1
		  and is_recurring = true
		  and status <> 'canceled'`, userID, reason); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return disabled, methodsDisabled, nil
}

type adminRevokeSubscriptionAccessRequest struct {
	UserID         int64  `json:"userId"`
	SubscriptionID int64  `json:"subscriptionId"`
	Reason         string `json:"reason"`
}

func (h Handler) AdminRevokeSubscriptionAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if _, ok := h.requireAdminSection(w, r, "billing", true); !ok {
		return
	}
	var req adminRevokeSubscriptionAccessRequest
	if err := decodeJSONStrict(w, r, 32<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if req.UserID <= 0 && req.SubscriptionID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "userId or subscriptionId is required"})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "admin_revoked"
	}
	result, err := h.revokeSubscriptionAccess(r.Context(), req.UserID, req.SubscriptionID, reason)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if result["subscriptionsRevoked"] == int64(0) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "active subscription not found"})
		return
	}
	result["ok"] = true
	writeJSON(w, http.StatusOK, result)
}

func (h Handler) revokeSubscriptionAccess(ctx context.Context, userID, subscriptionID int64, reason string) (map[string]any, error) {
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		update subscriptions s
		set status = 'canceled',
		    active_to = least(active_to, now()),
		    autopay_disabled_reason = $3,
		    updated_at = now()
		where ($1::bigint = 0 or s.user_id = $1)
		  and ($2::bigint = 0 or s.id = $2)
		  and s.status <> 'canceled'
		returning s.id, s.user_id, s.tariff_id`, userID, subscriptionID, reason)
	if err != nil {
		return nil, err
	}
	type revokedSubscription struct {
		id       int64
		userID   int64
		tariffID int64
	}
	revoked := []revokedSubscription{}
	for rows.Next() {
		var item revokedSubscription
		if err := rows.Scan(&item.id, &item.userID, &item.tariffID); err != nil {
			rows.Close()
			return nil, err
		}
		revoked = append(revoked, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(revoked) == 0 {
		return map[string]any{"subscriptionsRevoked": int64(0), "accessRevoked": int64(0), "autoRenewDisabled": int64(0), "paymentMethodsDisabled": int64(0)}, nil
	}

	var accessRevoked int64
	for _, item := range revoked {
		ct, err := tx.Exec(ctx, `
			update user_mode_access uma
			set active_to = least(active_to, now()),
			    updated_at = now()
			where uma.user_id = $1
			  and uma.access_type = 'subscription'
			  and uma.active_to > now()
			  and exists (
			    select 1
			    from invoices i
			    where i.id = uma.source_id
			      and i.user_id = $1
			      and i.tariff_id = $2
			  )`, item.userID, item.tariffID)
		if err != nil {
			return nil, err
		}
		accessRevoked += ct.RowsAffected()
	}

	ct, err := tx.Exec(ctx, `
		update autopay_subscriptions a
		set is_enabled = false,
		    cancel_reason = $2,
		    updated_at = now()
		where exists (
		  select 1 from subscriptions s
		  where s.id = a.subscription_id
		    and ($1::bigint = 0 or s.user_id = $1)
		    and ($3::bigint = 0 or s.id = $3)
		)
		  and a.is_enabled = true`, userID, reason, subscriptionID)
	if err != nil {
		return nil, err
	}
	autoRenewDisabled := ct.RowsAffected()

	ct, err = tx.Exec(ctx, `
		update payment_methods pm
		set status = 'disabled',
		    yookassa_payment_method_id = 'disabled:' || pm.id::text || ':' || extract(epoch from now())::bigint::text,
		    updated_at = now()
		where pm.status = 'active'
		  and exists (
		    select 1
		    from autopay_subscriptions a
		    join subscriptions s on s.id = a.subscription_id
		    where a.payment_method_id = pm.id
		      and ($1::bigint = 0 or s.user_id = $1)
		      and ($2::bigint = 0 or s.id = $2)
		  )`, userID, subscriptionID)
	if err != nil {
		return nil, err
	}
	paymentMethodsDisabled := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{
		"subscriptionsRevoked":   int64(len(revoked)),
		"accessRevoked":          accessRevoked,
		"autoRenewDisabled":      autoRenewDisabled,
		"paymentMethodsDisabled": paymentMethodsDisabled,
	}, nil
}

func (h Handler) startBillingRun(ctx context.Context, runKey, source string, dryRun bool) (int64, bool, map[string]any, error) {
	var runID int64
	var status string
	var totalsBytes []byte
	var startedAt time.Time
	if err := h.DB.QueryRow(ctx, `
		insert into billing_runs (run_key, run_type, source, dry_run, status, started_at, totals)
		values ($1, 'yookassa_daily', $2, $3, 'running', now(), '{}'::jsonb)
		on conflict (run_key) do update
		set status = case when billing_runs.status = 'succeeded' then billing_runs.status else 'running' end,
		    error = case when billing_runs.status = 'succeeded' then billing_runs.error else null end,
		    finished_at = case when billing_runs.status = 'succeeded' then billing_runs.finished_at else null end,
		    updated_at = now()
		returning id, status, totals, started_at`,
		runKey, source, dryRun).Scan(&runID, &status, &totalsBytes, &startedAt); err != nil {
		return 0, false, nil, err
	}
	report := map[string]any{
		"runId":     runID,
		"runKey":    runKey,
		"dryRun":    dryRun,
		"status":    status,
		"startedAt": startedAt,
	}
	totals := map[string]any{}
	if len(totalsBytes) > 0 {
		_ = json.Unmarshal(totalsBytes, &totals)
	}
	report["totals"] = totals
	return runID, status == "succeeded", report, nil
}

func (h Handler) finishBillingRun(ctx context.Context, runID int64, status string, totals map[string]any, errorText string) error {
	payload, err := json.Marshal(totals)
	if err != nil {
		return err
	}
	_, err = h.DB.Exec(ctx, `
		update billing_runs
		set status = $2,
		    totals = $3::jsonb,
		    error = nullif($4, ''),
		    finished_at = now(),
		    updated_at = now()
		where id = $1`, runID, status, string(payload), errorText)
	return err
}

func (h Handler) recordBillingRunItem(ctx context.Context, runID int64, step, status string, counts map[string]int, errorText string) error {
	payload, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	_, err = h.DB.Exec(ctx, `
		insert into billing_run_items (billing_run_id, step, status, counts, error, started_at, finished_at)
		values ($1, $2, $3, $4::jsonb, nullif($5, ''), now(), now())
		on conflict (billing_run_id, step) do update
		set status = excluded.status,
		    counts = excluded.counts,
		    error = excluded.error,
		    finished_at = now()`,
		runID, step, status, string(payload), errorText)
	return err
}

func (h Handler) countPendingYooKassaInvoices(ctx context.Context, limit int) (map[string]int, error) {
	result := map[string]int{"checked": 0, "paid": 0, "canceled": 0, "failed": 0, "skipped": 0}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	var checked int
	err := h.DB.QueryRow(ctx, `
		select count(*)
		from (
		  select 1
		  from invoices
		  where status = 'pending'
		    and nullif(yookassa_payment_id, '') is not null
		  order by created_at asc, id asc
		  limit $1
		) pending`, limit).Scan(&checked)
	result["checked"] = checked
	return result, err
}

func (h Handler) countDueYooKassaRenewals(ctx context.Context) (int, error) {
	var count int
	err := h.DB.QueryRow(ctx, `
		select count(*)
		from autopay_subscriptions a
		join subscriptions s on s.id = a.subscription_id
		join payment_methods pm on pm.id = a.payment_method_id
		where a.is_enabled = true
		  and s.is_recurring = true
		  and s.status in ('active', 'past_due')
		  and pm.status = 'active'
		  and (
		    (a.next_retry_at is null and s.active_to <= now())
		    or (a.next_retry_at is not null and a.next_retry_at <= now())
		  )
		  and a.consecutive_failures < a.max_retry_attempts
		  and not exists (
		    select 1 from recurring_payment_attempts r
		    where r.autopay_subscription_id = a.id
		      and r.status = 'pending'
		  )`).Scan(&count)
	return count, err
}

func (h Handler) markExhaustedYooKassaRenewals(ctx context.Context, dryRun bool) (map[string]int, error) {
	result := map[string]int{"exhausted": 0, "disabled": 0}
	if dryRun {
		var exhausted int
		err := h.DB.QueryRow(ctx, `
			select count(*)
			from autopay_subscriptions a
			join subscriptions s on s.id = a.subscription_id
			where a.is_enabled = true
			  and s.is_recurring = true
			  and s.status = 'past_due'
			  and a.consecutive_failures >= a.max_retry_attempts`).Scan(&exhausted)
		result["exhausted"] = exhausted
		return result, err
	}
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		update autopay_subscriptions a
		set is_enabled = false,
		    cancel_reason = 'max_retries_exhausted',
		    last_error = coalesce(nullif(a.last_error, ''), 'max_retries_exhausted'),
		    updated_at = now()
		from subscriptions s
		where s.id = a.subscription_id
		  and a.is_enabled = true
		  and s.is_recurring = true
		  and s.status = 'past_due'
		  and a.consecutive_failures >= a.max_retry_attempts
		returning a.subscription_id`)
	if err != nil {
		return result, err
	}
	var subscriptionIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		subscriptionIDs = append(subscriptionIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	result["exhausted"] = len(subscriptionIDs)
	result["disabled"] = len(subscriptionIDs)
	if len(subscriptionIDs) > 0 {
		// S-1: is_recurring is kept in sync with autopay_subscriptions.is_enabled;
		// otherwise subscriptions keeps a stale is_recurring=true after
		// autopay has actually been turned off (all renewal requests require
		// BOTH flags, so it causes no extra charge, but the mismatch
		// is misleading when reading the data directly).
		if _, err := tx.Exec(ctx, `
			update subscriptions
			set is_recurring = false
			where id = any($1)`, subscriptionIDs); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

type adminTestAutoChargeRequest struct {
	UserID         int64   `json:"userId"`
	SubscriptionID int64   `json:"subscriptionId"`
	AmountRub      float64 `json:"amountRub"`
	Description    string  `json:"description"`
}

func (h Handler) AdminYooKassaTestAutoCharge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	admin, ok := h.requireAdminSection(w, r, "billing", true)
	if !ok {
		return
	}
	var req adminTestAutoChargeRequest
	if err := decodeJSONStrict(w, r, 32<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if req.UserID <= 0 && req.SubscriptionID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "userId or subscriptionId is required"})
		return
	}
	if req.AmountRub <= 0 || math.IsNaN(req.AmountRub) || math.IsInf(req.AmountRub, 0) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "amount must be positive"})
		return
	}
	if req.AmountRub > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "test charge amount limit is 500 RUB"})
		return
	}

	result, err := h.createYooKassaTestAutoCharge(r.Context(), admin.ID, req)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "daily limit") {
			status = http.StatusTooManyRequests
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	result["ok"] = true
	writeJSON(w, http.StatusOK, result)
}

func (h Handler) createYooKassaTestAutoCharge(ctx context.Context, adminID int64, req adminTestAutoChargeRequest) (map[string]any, error) {
	marker := fmt.Sprintf("admin_test_charge:%d", adminID)

	type chargeTarget struct {
		autopayID             int64
		subscriptionID        int64
		userID                int64
		tariffID              int64
		paymentMethodRowID    int64
		yookassaPaymentMethod string
		months                int
	}
	var target chargeTarget
	err := h.DB.QueryRow(ctx, `
		select a.id, s.id, s.user_id, s.tariff_id, a.payment_method_id,
		       pm.yookassa_payment_method_id, coalesce(a.renewal_period_months, 1)
		from autopay_subscriptions a
		join subscriptions s on s.id = a.subscription_id
		join payment_methods pm on pm.id = a.payment_method_id
		where a.is_enabled = true
		  and s.status in ('active', 'past_due')
		  and s.is_recurring = true
		  and pm.status = 'active'
		  and ($1::bigint = 0 or s.user_id = $1)
		  and ($2::bigint = 0 or s.id = $2)
		order by s.active_to desc, s.id desc
		limit 1`, req.UserID, req.SubscriptionID).Scan(
		&target.autopayID,
		&target.subscriptionID,
		&target.userID,
		&target.tariffID,
		&target.paymentMethodRowID,
		&target.yookassaPaymentMethod,
		&target.months,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("active saved payment method not found")
	}
	if err != nil {
		return nil, err
	}
	if target.months <= 0 {
		target.months = 1
	}

	var userDailyCount, adminDailyCount int
	if err := h.DB.QueryRow(ctx, `
		select
		  count(*) filter (where user_id = $1),
		  count(*) filter (where cancellation_reason = $2)
		from invoices
		where cancellation_reason like 'admin_test_charge:%'
		  and created_at >= date_trunc('day', now())`, target.userID, marker).Scan(&userDailyCount, &adminDailyCount); err != nil {
		return nil, err
	}
	if userDailyCount >= 3 {
		return nil, fmt.Errorf("daily limit exceeded for user")
	}
	if adminDailyCount >= 3 {
		return nil, fmt.Errorf("daily limit exceeded for admin")
	}

	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = "Тестовое автосписание Mindstrata"
	}
	amount := fmt.Sprintf("%.2f", req.AmountRub)
	payment, err := h.createYooKassaPayment(ctx, map[string]any{
		"amount":            map[string]string{"value": amount, "currency": "RUB"},
		"capture":           true,
		"payment_method_id": target.yookassaPaymentMethod,
		"description":       description,
		"metadata": map[string]string{
			"kind":                    "admin_test_auto_charge",
			"admin_id":                fmt.Sprint(adminID),
			"user_id":                 fmt.Sprint(target.userID),
			"subscription_id":         fmt.Sprint(target.subscriptionID),
			"autopay_subscription_id": fmt.Sprint(target.autopayID),
		},
	}, fmt.Sprintf("admin-test-charge-%d-%d-%d", adminID, target.userID, time.Now().UnixNano()))
	if err != nil {
		return nil, err
	}
	_, err = h.DB.Exec(ctx, `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested,
			yookassa_payment_method_id, renewal_attempt, cancellation_reason
		)
		values ($1, $2, $3, $4::numeric, 'RUB', 'pending', $5, now() + interval '24 hours',
		        true, true, $6, 0, $7)
		on conflict (yookassa_payment_id) do nothing`,
		target.userID, target.tariffID, payment.ID, amount, target.months, target.yookassaPaymentMethod, marker)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"payment":        payment,
		"userId":         target.userID,
		"subscriptionId": target.subscriptionID,
		"amount":         amount,
		"currency":       "RUB",
		"remainingToday": map[string]int{
			"user":  2 - userDailyCount,
			"admin": 2 - adminDailyCount,
		},
	}, nil
}

func (h Handler) ProcessDueYooKassaRenewals(ctx context.Context, now time.Time) (int, error) {
	if h.DB == nil {
		return 0, nil
	}
	creds, err := h.activeYooKassaCredentials(ctx)
	if err != nil {
		return 0, err
	}
	if !creds.configured() {
		return 0, fmt.Errorf("yookassa is not configured")
	}
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		select a.id, s.id, s.user_id, s.tariff_id, a.payment_method_id,
		       pm.yookassa_payment_method_id, coalesce(s.amount_paid, t.monthly_price)::text, 'RUB',
		       coalesce(a.renewal_period_months, 1), s.active_to,
		       a.consecutive_failures, a.max_retry_attempts
		from autopay_subscriptions a
		join subscriptions s on s.id = a.subscription_id
		join payment_methods pm on pm.id = a.payment_method_id
		join tariffs t on t.id = s.tariff_id
		where a.is_enabled = true
		  and s.is_recurring = true
		  and s.status in ('active', 'past_due')
		  and pm.status = 'active'
		  and (
		    (a.next_retry_at is null and s.active_to <= $1)
		    or (a.next_retry_at is not null and a.next_retry_at <= $1)
		  )
		  and a.consecutive_failures < a.max_retry_attempts
		  and not exists (
		    select 1 from recurring_payment_attempts r
		    where r.autopay_subscription_id = a.id
		      and r.status = 'pending'
		  )
		order by s.active_to asc, a.id asc
		limit 25
		for update of a skip locked`, now)
	if err != nil {
		return 0, err
	}

	type dueRenewal struct {
		autopayID             int64
		subscriptionID        int64
		userID                int64
		tariffID              int64
		paymentMethodRowID    int64
		yookassaPaymentMethod string
		amount                string
		currency              string
		months                int
		activeTo              time.Time
		failures              int
		maxRetries            int
	}
	var due []dueRenewal
	for rows.Next() {
		var item dueRenewal
		if err := rows.Scan(&item.autopayID, &item.subscriptionID, &item.userID, &item.tariffID, &item.paymentMethodRowID, &item.yookassaPaymentMethod, &item.amount, &item.currency, &item.months, &item.activeTo, &item.failures, &item.maxRetries); err != nil {
			return 0, err
		}
		if item.months <= 0 {
			item.months = 1
		}
		if item.currency == "" {
			item.currency = "RUB"
		}
		due = append(due, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()

	processed := 0
	for _, item := range due {
		attempt := item.failures + 1
		renewalStart := item.activeTo
		if renewalStart.Before(now) {
			renewalStart = now
		}
		renewalEnd := renewalStart.AddDate(0, item.months, 0)
		idempotenceKey := fmt.Sprintf("renewal-%d-%d-%s", item.autopayID, attempt, renewalStart.UTC().Format("20060102"))
		payment, err := h.createYooKassaPayment(ctx, map[string]any{
			"amount":            map[string]string{"value": item.amount, "currency": item.currency},
			"capture":           true,
			"payment_method_id": item.yookassaPaymentMethod,
			"description":       fmt.Sprintf("Автопродление Mindstrata: тариф %d", item.tariffID),
			"metadata": map[string]string{
				"autopay_subscription_id": fmt.Sprint(item.autopayID),
				"subscription_id":         fmt.Sprint(item.subscriptionID),
				"user_id":                 fmt.Sprint(item.userID),
				"tariff_id":               fmt.Sprint(item.tariffID),
				"kind":                    "recurring_renewal",
			},
		}, idempotenceKey)
		if err != nil {
			_, _ = tx.Exec(ctx, `
				update autopay_subscriptions
				set consecutive_failures = consecutive_failures + 1,
				    last_attempt_at = now(),
				    next_retry_at = now() + make_interval(hours => retry_interval_hours),
				    grace_until = coalesce(grace_until, $2),
				    last_error = $3,
				    updated_at = now()
				where id = $1`, item.autopayID, now.Add(yookassaRenewalGrace), err.Error())
			_, _ = tx.Exec(ctx, `update subscriptions set status='past_due', updated_at=now() where id=$1`, item.subscriptionID)
			continue
		}
		_, err = tx.Exec(ctx, `
			insert into invoices (
				user_id, tariff_id, yookassa_payment_id, amount, currency, status,
				subscription_months, expires_at, is_recurring, autorenew_requested,
				yookassa_payment_method_id, renewal_attempt
			)
			values ($1, $2, $3, $4::numeric, $5, 'pending', $6, $7, true, true, $8, $9)
			on conflict (yookassa_payment_id) do nothing`,
			item.userID, item.tariffID, payment.ID, item.amount, item.currency, item.months, now.Add(24*time.Hour), item.yookassaPaymentMethod, attempt)
		if err != nil {
			return processed, err
		}
		_, err = tx.Exec(ctx, `
			insert into recurring_payment_attempts (
				autopay_subscription_id, yookassa_payment_id, amount, currency, attempt_number,
				status, renewal_start_date, renewal_end_date, original_renewal_start_date
			)
			values ($1, $2, $3::numeric, $4, $5, 'pending', $6, $7, $6)
			on conflict (yookassa_payment_id) do nothing`,
			item.autopayID, payment.ID, item.amount, item.currency, attempt, renewalStart, renewalEnd)
		if err != nil {
			return processed, err
		}
		_, _ = tx.Exec(ctx, `
			update autopay_subscriptions
			set last_attempt_at = now(),
			    next_retry_at = null,
			    updated_at = now()
			where id = $1`, item.autopayID)
		processed++
	}
	if err := tx.Commit(ctx); err != nil {
		return processed, err
	}
	return processed, nil
}

func (h Handler) enableAutoRenewFromPaidInvoice(ctx context.Context, tx pgx.Tx, invoiceID, userID, tariffID int64, paymentID string, amount string, subscriptionMonths int, activeTo time.Time, method yooKassaPaymentMethodData) error {
	paymentMethodID := strings.TrimSpace(method.ID)
	if paymentMethodID == "" || !method.Saved {
		return nil
	}
	methodType := strings.TrimSpace(method.Type)
	if methodType == "" {
		methodType = "bank_card"
	}
	var paymentMethodRowID int64
	if err := tx.QueryRow(ctx, `
		insert into payment_methods (user_id, yookassa_payment_method_id, payment_method_type, status, last_used_at, created_at, updated_at)
		values ($1, $2, $3, 'active', now(), now(), now())
		on conflict (yookassa_payment_method_id) do update
		set user_id = excluded.user_id,
		    payment_method_type = excluded.payment_method_type,
		    status = 'active',
		    last_used_at = now(),
		    updated_at = now()
		returning id`, userID, paymentMethodID, methodType).Scan(&paymentMethodRowID); err != nil {
		return err
	}

	var dailyLimit *int
	_ = tx.QueryRow(ctx, `select daily_message_limit from tariffs where id = $1`, tariffID).Scan(&dailyLimit)
	var subscriptionID int64
	err := tx.QueryRow(ctx, `
		select id from subscriptions
		where user_id = $1 and tariff_id = $2 and is_recurring = true and status <> 'canceled'
		order by id desc
		limit 1`, userID, tariffID).Scan(&subscriptionID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx, `
			insert into subscriptions (
				user_id, tariff_id, status, active_from, active_to, daily_message_limit,
				access_priority, payment_reference, amount_paid, autopay_renewal_months,
				is_recurring, created_at, updated_at
			)
			values ($1, $2, 'active', now(), $3, $4, 0, $5, $6::numeric, $7, true, now(), now())
			returning id`,
			userID, tariffID, activeTo, dailyLimit, paymentID, amount, subscriptionMonths).Scan(&subscriptionID)
		if err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			update subscriptions
			set status = 'active',
			    active_to = greatest(active_to, $2),
			    payment_reference = $3,
			    amount_paid = $4::numeric,
			    autopay_renewal_months = $5,
			    autopay_disabled_reason = null,
			    updated_at = now()
			where id = $1`, subscriptionID, activeTo, paymentID, amount, subscriptionMonths); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		insert into autopay_subscriptions (
			subscription_id, payment_method_id, is_enabled, renewal_period_months,
			max_retry_attempts, retry_interval_hours, consecutive_failures,
			last_success_at, grace_until, next_retry_at, last_error, cancel_reason,
			created_at, updated_at
		)
		values ($1, $2, true, $3, 3, 24, 0, now(), null, null, null, null, now(), now())
		on conflict (subscription_id) do update
		set payment_method_id = excluded.payment_method_id,
		    is_enabled = true,
		    renewal_period_months = excluded.renewal_period_months,
		    consecutive_failures = 0,
		    last_success_at = now(),
		    grace_until = null,
		    next_retry_at = null,
		    last_error = null,
		    cancel_reason = null,
		    updated_at = now()`,
		subscriptionID, paymentMethodRowID, subscriptionMonths); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		update invoices
		set yookassa_payment_method_id = $2,
		    updated_at = now()
		where id = $1`, invoiceID, paymentMethodID)
	return err
}

func (h Handler) handleRecurringPaymentSucceeded(ctx context.Context, tx pgx.Tx, paymentID string, activeTo time.Time) error {
	var autopayID int64
	var renewalEnd time.Time
	err := tx.QueryRow(ctx, `
		update recurring_payment_attempts
		set status = 'succeeded',
		    completed_at = now()
		where yookassa_payment_id = $1 and status = 'pending'
		returning autopay_subscription_id, renewal_end_date`, paymentID).Scan(&autopayID, &renewalEnd)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if renewalEnd.Before(activeTo) {
		renewalEnd = activeTo
	}
	_, err = tx.Exec(ctx, `
		update autopay_subscriptions
		set consecutive_failures = 0,
		    last_success_at = now(),
		    grace_until = null,
		    next_retry_at = null,
		    last_error = null,
		    updated_at = now()
		where id = $1`, autopayID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		update subscriptions s
		set status = 'active',
		    active_to = greatest(s.active_to, $2),
		    payment_reference = $3,
		    updated_at = now()
		from autopay_subscriptions a
		where a.id = $1 and s.id = a.subscription_id`, autopayID, renewalEnd, paymentID)
	return err
}

func (h Handler) handleRecurringPaymentCanceled(ctx context.Context, paymentID, reason string) error {
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var autopayID int64
	err = tx.QueryRow(ctx, `
		update recurring_payment_attempts
		set status = 'canceled',
		    error_code = nullif($2, ''),
		    completed_at = now()
		where yookassa_payment_id = $1 and status = 'pending'
		returning autopay_subscription_id`, paymentID, reason).Scan(&autopayID)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	if reason == "permission_revoked" {
		if _, err := tx.Exec(ctx, `
			update autopay_subscriptions
			set is_enabled = false,
			    cancel_reason = 'permission_revoked',
			    last_error = 'permission_revoked',
			    updated_at = now()
			where id = $1`, autopayID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			update payment_methods pm
			set status = 'disabled',
			    updated_at = now()
			from autopay_subscriptions a
			where a.id = $1 and pm.id = a.payment_method_id`, autopayID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			update subscriptions s
			set status = 'canceled',
			    autopay_disabled_reason = 'permission_revoked',
			    updated_at = now()
			from autopay_subscriptions a
			where a.id = $1 and s.id = a.subscription_id`, autopayID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		update autopay_subscriptions
		set consecutive_failures = consecutive_failures + 1,
		    next_retry_at = now() + make_interval(hours => retry_interval_hours),
		    grace_until = coalesce(grace_until, now() + ($2 * interval '1 second')),
		    last_error = nullif($3, ''),
		    updated_at = now()
		where id = $1`, autopayID, int(yookassaRenewalGrace.Seconds()), reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		update subscriptions s
		set status = 'past_due',
		    updated_at = now()
		from autopay_subscriptions a
		where a.id = $1 and s.id = a.subscription_id`, autopayID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
