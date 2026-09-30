package anonymizer

import (
	"testing"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/stretchr/testify/require"

	"mindstrata-stage1/api/internal/temporaltest"
)

func TestAnonymizationScheduleOptions(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ScheduleCron:     "0 3 1 * *",
		ScheduleTimeZone: "Europe/Moscow",
	}

	options := anonymizationScheduleOptions(cfg)

	require.Equal(t, AnonymizationScheduleID, options.ID)
	require.Equal(t, []string{"0 3 1 * *"}, options.Spec.CronExpressions)
	require.Equal(t, "Europe/Moscow", options.Spec.TimeZoneName)
	require.Equal(t, enumspb.SCHEDULE_OVERLAP_POLICY_SKIP, options.Overlap)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, AnonymizationWorkflowID, action.ID)
	require.Equal(t, GoTaskQueue, action.TaskQueue)

	// Regression 2026-06-08: a schedule without Args for a workflow with a parameter
	// sent both workers (prod and staging) into a restart loop.
	temporaltest.RequireScheduleArgsMatchWorkflow(t, options)
}

// Regression 2026-06-09: shared prod/staging queues meant tasks could be
// executed by the other environment's worker against the other database.
func TestAnonymizationScheduleOptions_StagingSuffix(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ScheduleCron:     "0 3 1 * *",
		ScheduleTimeZone: "Europe/Moscow",
		QueueSuffix:      "-staging",
	}

	options := anonymizationScheduleOptions(cfg)

	require.Equal(t, AnonymizationScheduleID+"-staging", options.ID)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, AnonymizationWorkflowID+"-staging", action.ID)
	require.Equal(t, "anonymizer-go-staging", action.TaskQueue)
}
