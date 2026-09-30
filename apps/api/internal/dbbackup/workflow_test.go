package dbbackup

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestDatabaseBackupWorkflow_Success(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	want := BackupResult{
		Key:       "db-backups/mindstrata-20260614-043000.sql.gz",
		SizeBytes: 9_000_000,
		Kept:      []string{"a", "b", "c"},
		Deleted:   []string{"old"},
	}
	env.RegisterActivityWithOptions(
		func(ctx context.Context) (BackupResult, error) { return want, nil },
		activity.RegisterOptions{Name: BackupActivity},
	)

	env.ExecuteWorkflow(DatabaseBackupWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var got BackupResult
	require.NoError(t, env.GetWorkflowResult(&got))
	require.Equal(t, want, got)
}

func TestDatabaseBackupWorkflow_ActivityError(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(
		func(ctx context.Context) (BackupResult, error) {
			return BackupResult{}, errors.New("pg_dump failed")
		},
		activity.RegisterOptions{Name: BackupActivity},
	)

	env.ExecuteWorkflow(DatabaseBackupWorkflow)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}
