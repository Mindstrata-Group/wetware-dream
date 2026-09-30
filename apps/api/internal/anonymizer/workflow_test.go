package anonymizer

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestAnonymizeMessagesWorkflow(t *testing.T) {
	t.Parallel()

	env := newAnonymizerWorkflowTestEnvironment(t)
	env.OnActivity("CountPendingMessages").Return(int64(3), nil).Once()
	env.OnActivity("GetMessageBatch").Return([]MessageRow{
		{ID: 1, Content: "Позвони Ивану"},
		{ID: 2, Content: "Email test@example.com"},
	}, nil).Once()
	env.OnActivity(AnonymizeTextsActivity, []string{"Позвони Ивану", "Email test@example.com"}).Return(AnonymizeResult{
		Texts: []string{"Позвони [ИМЯ]", "Email [EMAIL]"},
		Stats: map[string]int{"ИМЯ": 1, "EMAIL": 1},
	}, nil).Once()
	env.OnActivity("WriteAnonymizedBatch", []AnonymizedRow{
		{ID: 1, Content: "Позвони [ИМЯ]"},
		{ID: 2, Content: "Email [EMAIL]"},
	}).Return(nil).Once()
	env.OnActivity("GetMessageBatch").Return([]MessageRow{
		{ID: 3, Content: "Пиши @ivan_petrov"},
	}, nil).Once()
	env.OnActivity(AnonymizeTextsActivity, []string{"Пиши @ivan_petrov"}).Return(AnonymizeResult{
		Texts: []string{"Пиши [КОНТАКТ]"},
		Stats: map[string]int{"КОНТАКТ": 1},
	}, nil).Once()
	env.OnActivity("WriteAnonymizedBatch", []AnonymizedRow{
		{ID: 3, Content: "Пиши [КОНТАКТ]"},
	}).Return(nil).Once()
	env.OnActivity("GetMessageBatch").Return([]MessageRow{}, nil).Once()

	env.ExecuteWorkflow(AnonymizeMessagesWorkflow, WorkflowResult{})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result WorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, int64(3), result.Pending)
	require.Equal(t, 3, result.Processed)
	require.Equal(t, map[string]int{"ИМЯ": 1, "EMAIL": 1, "КОНТАКТ": 1}, result.Stats)

	env.AssertExpectations(t)
}

func TestAnonymizeMessagesWorkflowRejectsMismatchedPythonResult(t *testing.T) {
	t.Parallel()

	env := newAnonymizerWorkflowTestEnvironment(t)
	env.OnActivity("CountPendingMessages").Return(int64(2), nil).Once()
	env.OnActivity("GetMessageBatch").Return([]MessageRow{
		{ID: 1, Content: "Иван"},
		{ID: 2, Content: "Пётр"},
	}, nil).Once()
	env.OnActivity(AnonymizeTextsActivity, []string{"Иван", "Пётр"}).Return(AnonymizeResult{
		Texts: []string{"[ИМЯ]"},
		Stats: map[string]int{"ИМЯ": 1},
	}, nil).Once()

	env.ExecuteWorkflow(AnonymizeMessagesWorkflow, WorkflowResult{})

	require.True(t, env.IsWorkflowCompleted())
	require.ErrorContains(t, env.GetWorkflowError(), "python activity returned 1 texts for 2 messages")
	env.AssertExpectations(t)
}

func newAnonymizerWorkflowTestEnvironment(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(AnonymizeMessagesWorkflow)
	env.RegisterActivityWithOptions(func() (int64, error) { return 0, nil }, activity.RegisterOptions{Name: "CountPendingMessages"})
	env.RegisterActivityWithOptions(func() ([]MessageRow, error) { return nil, nil }, activity.RegisterOptions{Name: "GetMessageBatch"})
	env.RegisterActivityWithOptions(func(rows []AnonymizedRow) error { return nil }, activity.RegisterOptions{Name: "WriteAnonymizedBatch"})
	env.RegisterActivityWithOptions(func(texts []string) (AnonymizeResult, error) { return AnonymizeResult{}, nil }, activity.RegisterOptions{Name: AnonymizeTextsActivity})
	return env
}
