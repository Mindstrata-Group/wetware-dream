package dbbackup

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"mindstrata-stage1/api/internal/anonymizer"
)

// BackupResult is the backup result, visible in the Temporal UI.
// The fields match the BackupResult dataclass in apps/anonymizer/anonymizer/backup.py.
type BackupResult struct {
	Key       string   `json:"key"`
	SizeBytes int64    `json:"size_bytes"`
	Kept      []string `json:"kept"`
	Deleted   []string `json:"deleted"`
}

func DatabaseBackupWorkflow(ctx workflow.Context) (BackupResult, error) {
	// The Python queue is shared by prod/staging: the only python worker
	// holds the prod S3/DB credentials, so the backup schedule is enabled only on prod.
	opts := workflow.ActivityOptions{
		TaskQueue:           anonymizer.PythonTaskQueue,
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Minute,
			MaximumAttempts: 3,
		},
	}
	var result BackupResult
	err := workflow.ExecuteActivity(workflow.WithActivityOptions(ctx, opts), BackupActivity).Get(ctx, &result)
	return result, err
}
