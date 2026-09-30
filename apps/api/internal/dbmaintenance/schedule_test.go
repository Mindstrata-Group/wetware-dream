package dbmaintenance

import (
	"testing"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/stretchr/testify/require"

	"mindstrata-stage1/api/internal/anonymizer"
	"mindstrata-stage1/api/internal/temporaltest"
)

func TestModesMarkdownScheduleOptions(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ScheduleCron:     "0 4 1 * *",
		ScheduleTimeZone: "Asia/Yekaterinburg",
	}

	options := modesMarkdownScheduleOptions(cfg)

	require.Equal(t, ModesMarkdownScheduleID, options.ID)
	require.Equal(t, []string{"0 4 1 * *"}, options.Spec.CronExpressions)
	require.Equal(t, "Asia/Yekaterinburg", options.Spec.TimeZoneName)
	require.Equal(t, enumspb.SCHEDULE_OVERLAP_POLICY_SKIP, options.Overlap)
	require.True(t, options.PauseOnFailure)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, ModesMarkdownWorkflowID, action.ID)
	require.Equal(t, anonymizer.GoTaskQueue, action.TaskQueue)

	temporaltest.RequireScheduleArgsMatchWorkflow(t, options)
}

func TestModesMarkdownScheduleOptions_StagingSuffix(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ScheduleCron:     "0 4 1 * *",
		ScheduleTimeZone: "Asia/Yekaterinburg",
		QueueSuffix:      "-staging",
	}

	options := modesMarkdownScheduleOptions(cfg)

	require.Equal(t, ModesMarkdownScheduleID+"-staging", options.ID)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, ModesMarkdownWorkflowID+"-staging", action.ID)
	require.Equal(t, "anonymizer-go-staging", action.TaskQueue)
}
