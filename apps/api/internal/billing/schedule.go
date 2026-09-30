package billing

import (
	"context"
	"errors"
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

func DailyRenewalsScheduleOptions(cfg Config) client.ScheduleOptions {
	return client.ScheduleOptions{
		ID: DailyRenewalsScheduleID + cfg.QueueSuffix,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{cfg.RenewalScheduleCron},
			TimeZoneName:    cfg.RenewalScheduleTimeZone,
		},
		Action: &client.ScheduleWorkflowAction{
			ID:                       DailyRenewalsWorkflowID + cfg.QueueSuffix,
			Workflow:                 DailyYooKassaRenewalsWorkflow,
			Args:                     []any{DailyBillingInput{Source: "temporal"}},
			TaskQueue:                TaskQueue + cfg.QueueSuffix,
			WorkflowRunTimeout:       defaultRenewalRunTimeout,
			WorkflowTaskTimeout:      defaultRenewalTaskTimeout,
			StaticSummary:            "Daily YooKassa renewal billing",
			StaticDetails:            "Runs due subscription renewals and schedules failed payments for retry through database state.",
			WorkflowExecutionTimeout: defaultRenewalRunTimeout,
		},
		Overlap:            enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		CatchupWindow:      defaultRenewalCatchupWindow,
		PauseOnFailure:     false,
		Note:               fmt.Sprintf("Daily YooKassa due renewals: %s %s", cfg.RenewalScheduleCron, cfg.RenewalScheduleTimeZone),
		TriggerImmediately: false,
	}
}

func EnsureDailyRenewalsSchedule(ctx context.Context, temporalClient client.Client, cfg Config) error {
	if !cfg.RenewalScheduleEnabled {
		return nil
	}
	options := DailyRenewalsScheduleOptions(cfg)
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
							Note:   options.Note,
							Paused: options.Paused,
						},
					},
				}, nil
			},
		})
	}
	return nil
}
