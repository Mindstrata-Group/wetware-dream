package dbbackup

import (
	"context"
	"errors"
	"fmt"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"

	"mindstrata-stage1/api/internal/anonymizer"
)

func EnsureBackupSchedule(ctx context.Context, temporalClient client.Client, cfg Config) error {
	if !cfg.ScheduleEnabled {
		return nil
	}
	options := backupScheduleOptions(cfg)
	if _, err := temporalClient.ScheduleClient().Create(ctx, options); err != nil {
		if !errors.Is(err, temporal.ErrScheduleAlreadyRunning) {
			return err
		}
		return temporalClient.ScheduleClient().GetHandle(ctx, options.ID).Update(ctx, client.ScheduleUpdateOptions{
			DoUpdate: func(client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
				return &client.ScheduleUpdate{
					Schedule: &client.Schedule{
						Action: options.Action,
						Spec:   &options.Spec,
						Policy: &client.SchedulePolicies{
							Overlap:        options.Overlap,
							CatchupWindow:  options.CatchupWindow,
							PauseOnFailure: options.PauseOnFailure,
						},
						State: &client.ScheduleState{
							Note: options.Note,
						},
					},
				}, nil
			},
		})
	}
	return nil
}

func backupScheduleOptions(cfg Config) client.ScheduleOptions {
	return client.ScheduleOptions{
		ID: BackupScheduleID + cfg.QueueSuffix,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{cfg.ScheduleCron},
			TimeZoneName:    cfg.ScheduleTimeZone,
		},
		Action: &client.ScheduleWorkflowAction{
			ID:                       BackupWorkflowID + cfg.QueueSuffix,
			Workflow:                 DatabaseBackupWorkflow,
			TaskQueue:                anonymizer.GoTaskQueue + cfg.QueueSuffix,
			WorkflowRunTimeout:       2 * time.Hour,
			WorkflowExecutionTimeout: 2 * time.Hour,
			StaticSummary:            "Еженедельный оффсайт-бэкап БД в Timeweb S3",
			StaticDetails:            "pg_dump → gzip → s3.twcstorage.ru/db-backups, хранятся 3 последних дампа.",
		},
		Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		// Weekly backup: if Temporal was down at start time, catch up
		// within two days, otherwise wait for the next cycle.
		CatchupWindow:  48 * time.Hour,
		PauseOnFailure: true,
		Note:           fmt.Sprintf("Оффсайт-бэкап: %s %s", cfg.ScheduleCron, cfg.ScheduleTimeZone),
	}
}
