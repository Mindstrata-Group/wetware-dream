package dbmaintenance

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

func NormalizeModesMarkdownWorkflow(ctx workflow.Context) (ModesMarkdownResult, error) {
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	}
	var result ModesMarkdownResult
	err := workflow.ExecuteActivity(workflow.WithActivityOptions(ctx, opts), NormalizeModesActivity).Get(ctx, &result)
	return result, err
}
