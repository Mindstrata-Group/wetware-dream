package dbmaintenance

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

func EnsureModesMarkdownSchedule(ctx context.Context, temporalClient client.Client, cfg Config) error {
	if !cfg.ScheduleEnabled {
		return nil
	}
	options := modesMarkdownScheduleOptions(cfg)
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

func modesMarkdownScheduleOptions(cfg Config) client.ScheduleOptions {
	return client.ScheduleOptions{
		ID: ModesMarkdownScheduleID + cfg.QueueSuffix,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{cfg.ScheduleCron},
			TimeZoneName:    cfg.ScheduleTimeZone,
		},
		Action: &client.ScheduleWorkflowAction{
			ID:                       ModesMarkdownWorkflowID + cfg.QueueSuffix,
			Workflow:                 NormalizeModesMarkdownWorkflow,
			TaskQueue:                anonymizer.GoTaskQueue + cfg.QueueSuffix,
			WorkflowRunTimeout:       30 * time.Minute,
			WorkflowExecutionTimeout: 30 * time.Minute,
			StaticSummary:            "Ежемесячная очистка markdown-разметки в режимах",
			StaticDetails:            "Вызывает normalize_modes_markdown(): обновляет prompt, criteria и welcome_message; demo_chat не изменяет.",
		},
		Overlap:        enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		CatchupWindow:  48 * time.Hour,
		PauseOnFailure: true,
		Note:           fmt.Sprintf("Очистка markdown в modes: %s %s", cfg.ScheduleCron, cfg.ScheduleTimeZone),
	}
}
