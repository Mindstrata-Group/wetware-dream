package billing

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type RenewalRunResult struct {
	Processed int
	RunAt     time.Time
}

type DailyBillingInput struct {
	RunKey         string
	Source         string
	DryRun         bool
	ReconcileLimit int
}

type BillingRunStartResult struct {
	RunID            int64
	RunKey           string
	DryRun           bool
	AlreadySucceeded bool
	ExistingTotals   map[string]any
	StartedAt        time.Time
}

type BillingStepResult struct {
	Counts map[string]int
	RunAt  time.Time
}

type BillingRunFinishInput struct {
	RunID  int64
	Status string
	Totals map[string]any
	Error  string
}

type DailyBillingResult struct {
	RunID      int64
	RunKey     string
	DryRun     bool
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
	Reconcile  map[string]int
	Renewals   RenewalRunResult
	Exhausted  map[string]int
	Totals     map[string]any
}

func DailyYooKassaRenewalsWorkflow(ctx workflow.Context, input DailyBillingInput) (DailyBillingResult, error) {
	// TaskQueue is not set: activities go to the same (environment-suffixed)
	// queue the workflow itself runs on.
	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		ScheduleToCloseTimeout: 20 * time.Minute,
		StartToCloseTimeout:    10 * time.Minute,
		HeartbeatTimeout:       30 * time.Second,
		RetryPolicy:            &temporal.RetryPolicy{MaximumAttempts: 1},
	})

	if input.Source == "" {
		input.Source = "temporal"
	}
	if input.ReconcileLimit <= 0 {
		input.ReconcileLimit = 50
	}
	if input.RunKey == "" {
		input.RunKey = "yookassa-daily-" + workflow.Now(ctx).UTC().Format("2006-01-02")
	}

	var start BillingRunStartResult
	if err := workflow.ExecuteActivity(activityCtx, StartBillingRunActivity, input).Get(ctx, &start); err != nil {
		return DailyBillingResult{}, err
	}
	result := DailyBillingResult{
		RunID:     start.RunID,
		RunKey:    start.RunKey,
		DryRun:    start.DryRun,
		Status:    "running",
		StartedAt: start.StartedAt,
		Totals:    map[string]any{},
	}
	if start.AlreadySucceeded {
		result.Status = "succeeded"
		result.Totals = start.ExistingTotals
		workflow.GetLogger(ctx).Info("daily YooKassa billing run already succeeded", "runKey", start.RunKey)
		return result, nil
	}

	var workflowErr error
	defer func() {
		if workflowErr == nil {
			return
		}
		_ = workflow.ExecuteActivity(activityCtx, FinishBillingRunActivity, BillingRunFinishInput{
			RunID:  start.RunID,
			Status: "failed",
			Totals: result.Totals,
			Error:  workflowErr.Error(),
		}).Get(ctx, nil)
	}()

	// Process all pending invoices in batches of ReconcileLimit until none
	// are left (the activity returns checked=0).
	allReconcile := map[string]int{}
	for {
		var batch BillingStepResult
		if err := workflow.ExecuteActivity(activityCtx, ReconcileInvoicesActivity, start.RunID, input.ReconcileLimit, input.DryRun).Get(ctx, &batch); err != nil {
			workflowErr = err
			return DailyBillingResult{}, err
		}
		for k, v := range batch.Counts {
			allReconcile[k] += v
		}
		if batch.Counts["checked"] == 0 {
			break
		}
	}
	result.Reconcile = allReconcile

	var renewals RenewalRunResult
	if err := workflow.ExecuteActivity(activityCtx, RunDueRenewalsActivity, start.RunID, input.DryRun).Get(ctx, &renewals); err != nil {
		workflowErr = err
		return DailyBillingResult{}, err
	}
	result.Renewals = renewals

	var exhausted BillingStepResult
	if err := workflow.ExecuteActivity(activityCtx, MarkExhaustedRenewalsActivity, start.RunID, input.DryRun).Get(ctx, &exhausted); err != nil {
		workflowErr = err
		return DailyBillingResult{}, err
	}
	result.Exhausted = exhausted.Counts
	result.Totals = map[string]any{
		"reconcile": result.Reconcile,
		"renewals":  map[string]any{"processed": result.Renewals.Processed},
		"exhausted": result.Exhausted,
	}

	if err := workflow.ExecuteActivity(activityCtx, FinishBillingRunActivity, BillingRunFinishInput{
		RunID:  start.RunID,
		Status: "succeeded",
		Totals: result.Totals,
	}).Get(ctx, nil); err != nil {
		workflowErr = err
		return DailyBillingResult{}, err
	}
	result.Status = "succeeded"
	result.FinishedAt = workflow.Now(ctx).UTC()
	workflow.GetLogger(ctx).Info("daily YooKassa billing finished", "runKey", result.RunKey, "processed", result.Renewals.Processed)
	return result, nil
}
