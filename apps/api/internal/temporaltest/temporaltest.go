// Package temporaltest checks that Temporal schedules are consistent with workflows.
//
// Temporal validates that Args match the workflow signature only at runtime,
// when the schedule is created. If they diverge, the worker crashes on start with
// an error like "expected 1 args for function ... but found 0" and enters a restart loop
// (this took prod down on 2026-06-08 after the AnonymizeMessagesWorkflow signature changed).
// These checks catch the divergence at unit-test time.
package temporaltest

import (
	"reflect"
	"testing"

	"go.temporal.io/sdk/client"

	"github.com/stretchr/testify/require"
)

// RequireScheduleArgsMatchWorkflow checks that the number and types of arguments
// in ScheduleWorkflowAction match the workflow function signature.
func RequireScheduleArgsMatchWorkflow(t *testing.T, options client.ScheduleOptions) {
	t.Helper()

	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	require.True(t, ok, "schedule action должен быть *client.ScheduleWorkflowAction")
	require.NotNil(t, action.Workflow, "workflow в schedule action не задан")

	fnType := reflect.TypeOf(action.Workflow)
	if fnType.Kind() != reflect.Func {
		// The workflow is given by its string name: the signature cannot be checked.
		return
	}

	// The workflow's first parameter is workflow.Context; the schedule does not pass it.
	wantArgs := fnType.NumIn() - 1
	require.Equal(t, wantArgs, len(action.Args),
		"схедула %q передаёт %d аргументов, а воркфлоу ожидает %d — воркер упадёт на старте",
		options.ID, len(action.Args), wantArgs)

	for i, arg := range action.Args {
		paramType := fnType.In(i + 1)
		require.NotNil(t, arg, "аргумент %d схедулы %q равен nil", i, options.ID)
		argType := reflect.TypeOf(arg)
		require.Truef(t, argType.AssignableTo(paramType),
			"аргумент %d схедулы %q имеет тип %s, а воркфлоу ожидает %s",
			i, options.ID, argType, paramType)
	}
}
