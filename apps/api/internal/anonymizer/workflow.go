package anonymizer

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

func AnonymizeMessagesWorkflow(ctx workflow.Context, acc WorkflowResult) (WorkflowResult, error) {
	var cfg WorkflowConfig
	if err := workflow.SideEffect(ctx, func(workflow.Context) any {
		return WorkflowConfigFromEnv()
	}).Get(&cfg); err != nil {
		return acc, err
	}
	logger := workflow.GetLogger(ctx)

	// TaskQueue is not set: Go activities (database access!) go to the same
	// environment-suffixed queue the workflow runs on, so a staging workflow
	// never lands on the prod worker and vice versa.
	goCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Minute,
		StartToCloseTimeout:    2 * time.Minute,
		HeartbeatTimeout:       30 * time.Second,
	})

	var total int64
	if err := workflow.ExecuteActivity(goCtx, "CountPendingMessages").Get(ctx, &total); err != nil {
		return acc, err
	}
	if acc.Pending == 0 {
		acc.Pending = total
	}
	if acc.Stats == nil {
		acc.Stats = make(map[string]int)
	}
	logger.Info("anonymization started", "pending", total)

	for {
		var batch []MessageRow
		if err := workflow.ExecuteActivity(goCtx, "GetMessageBatch").Get(ctx, &batch); err != nil {
			return acc, err
		}
		if len(batch) == 0 {
			break
		}

		texts := make([]string, len(batch))
		for i, row := range batch {
			texts[i] = row.Content
		}

		// The Python queue is shared by prod/staging: anonymize_texts is a pure
		// function (texts in, texts out) and never touches the database.
		pythonCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			TaskQueue:              PythonTaskQueue,
			ScheduleToCloseTimeout: 5 * time.Minute,
			HeartbeatTimeout:       30 * time.Second,
		})
		var result AnonymizeResult
		if err := workflow.ExecuteActivity(pythonCtx, AnonymizeTextsActivity, texts).Get(ctx, &result); err != nil {
			return acc, err
		}
		if len(result.Texts) != len(batch) {
			return acc, fmt.Errorf("python activity returned %d texts for %d messages", len(result.Texts), len(batch))
		}
		if len(result.Stats) > 0 {
			logger.Info("batch anonymized", "count", len(batch), "stats", result.Stats)
		}

		rows := make([]AnonymizedRow, len(batch))
		for i, row := range batch {
			rows[i] = AnonymizedRow{ID: row.ID, Content: result.Texts[i]}
		}
		if err := workflow.ExecuteActivity(goCtx, "WriteAnonymizedBatch", rows).Get(ctx, nil); err != nil {
			return acc, err
		}

		acc.Processed += len(batch)
		for k, v := range result.Stats {
			acc.Stats[k] += v
		}

		if workflow.GetInfo(ctx).GetContinueAsNewSuggested() {
			logger.Info("continuing as new to avoid history size limit", "processed", acc.Processed)
			return WorkflowResult{}, workflow.NewContinueAsNewError(ctx, AnonymizeMessagesWorkflow, acc)
		}
		if cfg.SleepBetweenBatches > 0 {
			if err := workflow.Sleep(ctx, cfg.SleepBetweenBatches); err != nil {
				return acc, err
			}
		}
	}

	logger.Info("anonymization finished", "pending", acc.Pending, "processed", acc.Processed, "stats", acc.Stats)
	return acc, nil
}
