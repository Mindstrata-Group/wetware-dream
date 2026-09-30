package billing

import (
	"testing"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/stretchr/testify/require"

	"mindstrata-stage1/api/internal/temporaltest"
)

func TestDailyRenewalsScheduleOptions(t *testing.T) {
	t.Parallel()

	cfg := Config{
		RenewalScheduleCron:     "15 4 * * *",
		RenewalScheduleTimeZone: "Europe/Moscow",
	}

	options := DailyRenewalsScheduleOptions(cfg)

	require.Equal(t, DailyRenewalsScheduleID, options.ID)
	require.Equal(t, []string{"15 4 * * *"}, options.Spec.CronExpressions)
	require.Equal(t, "Europe/Moscow", options.Spec.TimeZoneName)
	require.Equal(t, enumspb.SCHEDULE_OVERLAP_POLICY_SKIP, options.Overlap)
	require.Equal(t, defaultRenewalCatchupWindow, options.CatchupWindow)
	require.False(t, options.TriggerImmediately)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, DailyRenewalsWorkflowID, action.ID)
	require.Equal(t, TaskQueue, action.TaskQueue)

	temporaltest.RequireScheduleArgsMatchWorkflow(t, options)
}

// Regression 2026-06-09: prod and staging workers polled the same
// billing-renewals queue, so staging could run prod renewals against the staging DB.
func TestDailyRenewalsScheduleOptions_StagingSuffix(t *testing.T) {
	t.Parallel()

	cfg := Config{
		RenewalScheduleCron:     "15 4 * * *",
		RenewalScheduleTimeZone: "Europe/Moscow",
		QueueSuffix:             "-staging",
	}

	options := DailyRenewalsScheduleOptions(cfg)

	require.Equal(t, DailyRenewalsScheduleID+"-staging", options.ID)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, DailyRenewalsWorkflowID+"-staging", action.ID)
	require.Equal(t, "billing-renewals-staging", action.TaskQueue)
}
