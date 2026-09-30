package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"mindstrata-stage1/api/internal/httpapi"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Activities struct {
	DB                *pgxpool.Pool
	HTTPClient        *http.Client
	YooKassaShopID    string
	YooKassaSecretKey string
}

func NewActivities(db *pgxpool.Pool, cfg Config) *Activities {
	return &Activities{
		DB:                db,
		YooKassaShopID:    cfg.YooKassaShopID,
		YooKassaSecretKey: cfg.YooKassaSecretKey,
	}
}

func (a *Activities) StartBillingRun(ctx context.Context, input DailyBillingInput) (BillingRunStartResult, error) {
	if a.DB == nil {
		return BillingRunStartResult{}, fmt.Errorf("database is not configured")
	}
	if input.Source == "" {
		input.Source = "temporal"
	}
	if input.RunKey == "" {
		input.RunKey = "yookassa-daily-" + time.Now().UTC().Format("2006-01-02")
	}
	var result BillingRunStartResult
	var status string
	var totalsBytes []byte
	err := a.DB.QueryRow(ctx, `
		insert into billing_runs (run_key, run_type, source, dry_run, status, started_at, totals)
		values ($1, 'yookassa_daily', $2, $3, 'running', now(), '{}'::jsonb)
		on conflict (run_key) do update
		set status = case when billing_runs.status = 'succeeded' then billing_runs.status else 'running' end,
		    error = case when billing_runs.status = 'succeeded' then billing_runs.error else null end,
		    finished_at = case when billing_runs.status = 'succeeded' then billing_runs.finished_at else null end,
		    updated_at = now()
		returning id, run_key, dry_run, status, totals, started_at`,
		input.RunKey, input.Source, input.DryRun).Scan(
		&result.RunID,
		&result.RunKey,
		&result.DryRun,
		&status,
		&totalsBytes,
		&result.StartedAt,
	)
	if err != nil {
		return BillingRunStartResult{}, err
	}
	result.AlreadySucceeded = status == "succeeded"
	result.ExistingTotals = map[string]any{}
	if len(totalsBytes) > 0 {
		_ = json.Unmarshal(totalsBytes, &result.ExistingTotals)
	}
	return result, nil
}

func (a *Activities) ReconcilePendingYooKassaInvoices(ctx context.Context, runID int64, limit int, dryRun bool) (BillingStepResult, error) {
	if dryRun {
		counts, err := a.countPendingInvoices(ctx, limit)
		if err != nil {
			_ = a.recordBillingStep(ctx, runID, "reconcile_pending_invoices", "failed", counts, err.Error())
			return BillingStepResult{}, err
		}
		if err := a.recordBillingStep(ctx, runID, "reconcile_pending_invoices", "skipped", counts, "dry_run"); err != nil {
			return BillingStepResult{}, err
		}
		return BillingStepResult{Counts: counts, RunAt: time.Now().UTC()}, nil
	}
	handler := httpapi.Handler{
		DB:                a.DB,
		HTTPClient:        a.HTTPClient,
		YooKassaShopID:    a.YooKassaShopID,
		YooKassaSecretKey: a.YooKassaSecretKey,
	}
	counts, err := handler.ReconcilePendingYooKassaInvoices(ctx, limit)
	if err != nil {
		_ = a.recordBillingStep(ctx, runID, "reconcile_pending_invoices", "failed", counts, err.Error())
		return BillingStepResult{}, err
	}
	if err := a.recordBillingStep(ctx, runID, "reconcile_pending_invoices", "succeeded", counts, ""); err != nil {
		return BillingStepResult{}, err
	}
	return BillingStepResult{Counts: counts, RunAt: time.Now().UTC()}, nil
}

func (a *Activities) RunDueYooKassaRenewals(ctx context.Context, runID int64, dryRun bool) (RenewalRunResult, error) {
	if dryRun {
		count, err := a.countDueRenewals(ctx)
		counts := map[string]int{"due": count}
		if err != nil {
			_ = a.recordBillingStep(ctx, runID, "run_due_renewals", "failed", counts, err.Error())
			return RenewalRunResult{}, err
		}
		if err := a.recordBillingStep(ctx, runID, "run_due_renewals", "skipped", counts, "dry_run"); err != nil {
			return RenewalRunResult{}, err
		}
		return RenewalRunResult{Processed: 0, RunAt: time.Now().UTC()}, nil
	}
	handler := httpapi.Handler{
		DB:                a.DB,
		HTTPClient:        a.HTTPClient,
		YooKassaShopID:    a.YooKassaShopID,
		YooKassaSecretKey: a.YooKassaSecretKey,
	}
	processed, err := handler.ProcessDueYooKassaRenewals(ctx, time.Now())
	if err != nil {
		_ = a.recordBillingStep(ctx, runID, "run_due_renewals", "failed", map[string]int{"processed": processed}, err.Error())
		return RenewalRunResult{}, err
	}
	if err := a.recordBillingStep(ctx, runID, "run_due_renewals", "succeeded", map[string]int{"processed": processed}, ""); err != nil {
		return RenewalRunResult{}, err
	}
	return RenewalRunResult{Processed: processed, RunAt: time.Now().UTC()}, nil
}

func (a *Activities) MarkExhaustedRenewals(ctx context.Context, runID int64, dryRun bool) (BillingStepResult, error) {
	counts, err := a.markExhaustedRenewals(ctx, dryRun)
	if err != nil {
		_ = a.recordBillingStep(ctx, runID, "mark_exhausted_renewals", "failed", counts, err.Error())
		return BillingStepResult{}, err
	}
	status := "succeeded"
	errorText := ""
	if dryRun {
		status = "skipped"
		errorText = "dry_run"
	}
	if err := a.recordBillingStep(ctx, runID, "mark_exhausted_renewals", status, counts, errorText); err != nil {
		return BillingStepResult{}, err
	}
	return BillingStepResult{Counts: counts, RunAt: time.Now().UTC()}, nil
}

func (a *Activities) FinishBillingRun(ctx context.Context, input BillingRunFinishInput) error {
	if a.DB == nil {
		return fmt.Errorf("database is not configured")
	}
	totals, err := json.Marshal(input.Totals)
	if err != nil {
		return err
	}
	if input.Status == "" {
		input.Status = "succeeded"
	}
	_, err = a.DB.Exec(ctx, `
		update billing_runs
		set status = $2,
		    totals = $3::jsonb,
		    error = nullif($4, ''),
		    finished_at = now(),
		    updated_at = now()
		where id = $1`, input.RunID, input.Status, string(totals), input.Error)
	return err
}

func (a *Activities) countPendingInvoices(ctx context.Context, limit int) (map[string]int, error) {
	counts := map[string]int{"checked": 0, "paid": 0, "canceled": 0, "failed": 0, "skipped": 0}
	if a.DB == nil {
		return counts, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var checked int
	err := a.DB.QueryRow(ctx, `
		select count(*)
		from (
		  select 1
		  from invoices
		  where status = 'pending'
		    and nullif(yookassa_payment_id, '') is not null
		  order by created_at asc, id asc
		  limit $1
		) pending`, limit).Scan(&checked)
	counts["checked"] = checked
	return counts, err
}

func (a *Activities) countDueRenewals(ctx context.Context) (int, error) {
	if a.DB == nil {
		return 0, nil
	}
	var count int
	err := a.DB.QueryRow(ctx, `
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

func (a *Activities) markExhaustedRenewals(ctx context.Context, dryRun bool) (map[string]int, error) {
	counts := map[string]int{"exhausted": 0, "disabled": 0}
	if a.DB == nil {
		return counts, nil
	}
	if dryRun {
		var exhausted int
		err := a.DB.QueryRow(ctx, `
			select count(*)
			from autopay_subscriptions a
			join subscriptions s on s.id = a.subscription_id
			where a.is_enabled = true
			  and s.is_recurring = true
			  and s.status = 'past_due'
			  and a.consecutive_failures >= a.max_retry_attempts`).Scan(&exhausted)
		counts["exhausted"] = exhausted
		return counts, err
	}
	ct, err := a.DB.Exec(ctx, `
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
		  and a.consecutive_failures >= a.max_retry_attempts`)
	if err != nil {
		return counts, err
	}
	counts["exhausted"] = int(ct.RowsAffected())
	counts["disabled"] = int(ct.RowsAffected())
	return counts, nil
}

func (a *Activities) recordBillingStep(ctx context.Context, runID int64, step, status string, counts map[string]int, errorText string) error {
	if a.DB == nil || runID == 0 {
		return nil
	}
	payload, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	_, err = a.DB.Exec(ctx, `
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
