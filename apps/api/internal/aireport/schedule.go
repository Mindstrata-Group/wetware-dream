package aireport

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

func EnsureDailyReportSchedule(ctx context.Context, temporalClient client.Client, cfg Config) error {
	if !cfg.ScheduleEnabled {
		return nil
	}
	options := dailyReportScheduleOptions(cfg)
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

func dailyReportScheduleOptions(cfg Config) client.ScheduleOptions {
	return client.ScheduleOptions{
		ID: ScheduleID + cfg.QueueSuffix,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{cfg.ScheduleCron},
			TimeZoneName:    cfg.ScheduleTimeZone,
		},
		Action: &client.ScheduleWorkflowAction{
			ID:                       WorkflowID + cfg.QueueSuffix,
			Workflow:                 DailyAIProviderReportWorkflow,
			TaskQueue:                anonymizer.GoTaskQueue + cfg.QueueSuffix,
			WorkflowRunTimeout:       2 * time.Minute,
			WorkflowExecutionTimeout: 2 * time.Minute,
			StaticSummary:            "Ежедневный отчёт по AI-провайдерам в Uptime Kuma",
			StaticDetails:            "Считает ai_provider_events за вчера (сообщения по провайдерам, упавшие попытки, полные отказы цепочки) и пушит heartbeat в Kuma push-monitor.",
		},
		Overlap:        enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		CatchupWindow:  6 * time.Hour,
		PauseOnFailure: false,
		Note:           fmt.Sprintf("AI provider daily report: %s %s", cfg.ScheduleCron, cfg.ScheduleTimeZone),
	}
}
