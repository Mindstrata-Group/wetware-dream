package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestDailyYooKassaRenewalsWorkflowRunsFiveStepBillingPlan(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DailyYooKassaRenewalsWorkflow)
	registerBillingWorkflowTestActivities(env)
	runAt := time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC)
	env.SetStartTime(runAt)

	env.OnActivity(StartBillingRunActivity, mock.Anything).Return(BillingRunStartResult{
		RunID:     42,
		RunKey:    "yookassa-daily-2026-06-05",
		DryRun:    false,
		StartedAt: runAt,
	}, nil).Once()
	env.OnActivity(ReconcileInvoicesActivity, int64(42), 50, false).Return(BillingStepResult{
		Counts: map[string]int{"checked": 2, "paid": 1, "canceled": 0, "failed": 0, "skipped": 1},
		RunAt:  runAt,
	}, nil).Once()
	// Second batch: no more invoices, the loop ends.
	env.OnActivity(ReconcileInvoicesActivity, int64(42), 50, false).Return(BillingStepResult{
		Counts: map[string]int{"checked": 0},
		RunAt:  runAt,
	}, nil).Once()
	env.OnActivity(RunDueRenewalsActivity, int64(42), false).Return(RenewalRunResult{
		Processed: 3,
		RunAt:     runAt,
	}, nil).Once()
	env.OnActivity(MarkExhaustedRenewalsActivity, int64(42), false).Return(BillingStepResult{
		Counts: map[string]int{"exhausted": 1, "disabled": 1},
		RunAt:  runAt,
	}, nil).Once()
	env.OnActivity(FinishBillingRunActivity, mock.MatchedBy(func(input BillingRunFinishInput) bool {
		return input.RunID == 42 && input.Status == "succeeded"
	})).Return(nil).Once()

	env.ExecuteWorkflow(DailyYooKassaRenewalsWorkflow, DailyBillingInput{Source: "temporal"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)

	var result DailyBillingResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, int64(42), result.RunID)
	require.Equal(t, 3, result.Renewals.Processed)
	require.Equal(t, 1, result.Exhausted["disabled"])
	require.Equal(t, "succeeded", result.Status)
}

func TestDailyYooKassaRenewalsWorkflowSkipsAlreadySucceededRun(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DailyYooKassaRenewalsWorkflow)
	registerBillingWorkflowTestActivities(env)
	env.SetStartTime(time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC))
	env.OnActivity(StartBillingRunActivity, mock.Anything).Return(BillingRunStartResult{
		RunID:            42,
		RunKey:           "yookassa-daily-2026-06-05",
		AlreadySucceeded: true,
		ExistingTotals:   map[string]any{"renewals": map[string]any{"processed": float64(1)}},
	}, nil).Once()

	env.ExecuteWorkflow(DailyYooKassaRenewalsWorkflow, DailyBillingInput{Source: "temporal"})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestDailyYooKassaRenewalsWorkflowMarksRunFailedOnActivityError(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DailyYooKassaRenewalsWorkflow)
	registerBillingWorkflowTestActivities(env)
	env.SetStartTime(time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC))
	env.OnActivity(StartBillingRunActivity, mock.Anything).Return(BillingRunStartResult{
		RunID:  42,
		RunKey: "yookassa-daily-2026-06-05",
	}, nil).Once()
	env.OnActivity(ReconcileInvoicesActivity, int64(42), 50, false).Return(BillingStepResult{}, assertErr("yookassa is not configured")).Once()
	env.OnActivity(FinishBillingRunActivity, mock.MatchedBy(func(input BillingRunFinishInput) bool {
		return input.RunID == 42 && input.Status == "failed" && input.Error != ""
	})).Return(nil).Once()

	env.ExecuteWorkflow(DailyYooKassaRenewalsWorkflow, DailyBillingInput{Source: "temporal"})

	require.True(t, env.IsWorkflowCompleted())
	require.ErrorContains(t, env.GetWorkflowError(), "yookassa is not configured")
	env.AssertExpectations(t)
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}

func registerBillingWorkflowTestActivities(env *testsuite.TestWorkflowEnvironment) {
	env.RegisterActivityWithOptions(func(DailyBillingInput) (BillingRunStartResult, error) {
		return BillingRunStartResult{}, nil
	}, activity.RegisterOptions{Name: StartBillingRunActivity})
	env.RegisterActivityWithOptions(func(int64, int, bool) (BillingStepResult, error) {
		return BillingStepResult{}, nil
	}, activity.RegisterOptions{Name: ReconcileInvoicesActivity})
	env.RegisterActivityWithOptions(func(int64, bool) (RenewalRunResult, error) {
		return RenewalRunResult{}, nil
	}, activity.RegisterOptions{Name: RunDueRenewalsActivity})
	env.RegisterActivityWithOptions(func(int64, bool) (BillingStepResult, error) {
		return BillingStepResult{}, nil
	}, activity.RegisterOptions{Name: MarkExhaustedRenewalsActivity})
	env.RegisterActivityWithOptions(func(BillingRunFinishInput) error {
		return nil
	}, activity.RegisterOptions{Name: FinishBillingRunActivity})
}
