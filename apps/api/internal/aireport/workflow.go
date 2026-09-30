package aireport

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

func DailyAIProviderReportWorkflow(ctx workflow.Context) (ReportResult, error) {
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	}
	var result ReportResult
	err := workflow.ExecuteActivity(workflow.WithActivityOptions(ctx, opts), ReportActivity).Get(ctx, &result)
	return result, err
}
