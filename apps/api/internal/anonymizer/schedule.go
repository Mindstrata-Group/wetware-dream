package anonymizer

import (
	"context"
	"errors"
	"fmt"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

func EnsureAnonymizationSchedule(ctx context.Context, temporalClient client.Client, cfg Config) error {
	if !cfg.ScheduleEnabled {
		return nil
	}
	options := anonymizationScheduleOptions(cfg)
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

func anonymizationScheduleOptions(cfg Config) client.ScheduleOptions {
	return client.ScheduleOptions{
		ID: AnonymizationScheduleID + cfg.QueueSuffix,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{cfg.ScheduleCron},
			TimeZoneName:    cfg.ScheduleTimeZone,
		},
		Action: &client.ScheduleWorkflowAction{
			ID:                       AnonymizationWorkflowID + cfg.QueueSuffix,
			Workflow:                 AnonymizeMessagesWorkflow,
			Args:                     []any{WorkflowResult{}},
			TaskQueue:                GoTaskQueue + cfg.QueueSuffix,
			WorkflowRunTimeout:       24 * time.Hour,
			WorkflowExecutionTimeout: 24 * time.Hour,
			StaticSummary:            "Наташа — ежемесячная анонимизация сообщений",
			StaticDetails:            "Заменяет ПДн в старых сообщениях на [ИМЯ]/[ТЕЛЕФОН]/… через yargy NLP + regex.",
		},
		Overlap:        enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		CatchupWindow:  72 * time.Hour,
		PauseOnFailure: true,
		Note:           fmt.Sprintf("Наташа: %s %s", cfg.ScheduleCron, cfg.ScheduleTimeZone),
	}
}
