package dbbackup

import (
	"testing"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/stretchr/testify/require"

	"mindstrata-stage1/api/internal/anonymizer"
	"mindstrata-stage1/api/internal/temporaltest"
)

func TestBackupScheduleOptions(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ScheduleCron:     "30 4 * * 0",
		ScheduleTimeZone: "Asia/Yekaterinburg",
	}

	options := backupScheduleOptions(cfg)

	require.Equal(t, BackupScheduleID, options.ID)
	require.Equal(t, []string{"30 4 * * 0"}, options.Spec.CronExpressions)
	require.Equal(t, "Asia/Yekaterinburg", options.Spec.TimeZoneName)
	require.Equal(t, enumspb.SCHEDULE_OVERLAP_POLICY_SKIP, options.Overlap)
	require.True(t, options.PauseOnFailure)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, BackupWorkflowID, action.ID)
	require.Equal(t, anonymizer.GoTaskQueue, action.TaskQueue)

	temporaltest.RequireScheduleArgsMatchWorkflow(t, options)
}

func TestBackupScheduleOptions_StagingSuffix(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ScheduleCron:     "30 4 * * 0",
		ScheduleTimeZone: "Asia/Yekaterinburg",
		QueueSuffix:      "-staging",
	}

	options := backupScheduleOptions(cfg)

	require.Equal(t, BackupScheduleID+"-staging", options.ID)

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok)
	require.Equal(t, BackupWorkflowID+"-staging", action.ID)
	require.Equal(t, "anonymizer-go-staging", action.TaskQueue)
}
