//go:build integration

package httpapi

import (
	"context"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/anonymizer"
	"mindstrata-stage1/api/internal/testsupport"

	"github.com/stretchr/testify/require"
)

func TestAnonymizerActivities_GetWriteAndSkipOptedOut(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})

	allowed := f.CreateUser(TestUserOpts{})
	allowedDialog := f.CreateDialog(allowed.ID, mode.ID)
	allowedMessageID := f.AppendMessage(allowedDialog.ID, "user", "Иван +7 999 000 00 00")
	_, err := env.Pool.Exec(context.Background(), `update dialogs_messages set created_at = now() - interval '40 days' where id = $1`, allowedMessageID)
	require.NoError(t, err)

	optedOut := f.CreateUser(TestUserOpts{})
	_, err = env.Pool.Exec(context.Background(), `update users set allow_message_anonymization = false where id = $1`, optedOut.ID)
	require.NoError(t, err)
	optedOutDialog := f.CreateDialog(optedOut.ID, mode.ID)
	optedOutMessageID := f.AppendMessage(optedOutDialog.ID, "assistant", "Пётр test@example.com")
	_, err = env.Pool.Exec(context.Background(), `update dialogs_messages set created_at = now() - interval '40 days' where id = $1`, optedOutMessageID)
	require.NoError(t, err)

	recentMessageID := f.AppendMessage(allowedDialog.ID, "assistant", "Свежий email recent@example.com")
	_, err = env.Pool.Exec(context.Background(), `update dialogs_messages set created_at = now() - interval '1 day' where id = $1`, recentMessageID)
	require.NoError(t, err)

	activities := anonymizer.NewActivities(env.Pool, anonymizer.Config{MaxBatchBytes: 10 * 1024, MaxBatchRows: 10, MessageAgeDays: 30})
	batch, err := activities.GetMessageBatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, []anonymizer.MessageRow{{ID: allowedMessageID, Content: "Иван +7 999 000 00 00"}}, batch)

	require.NoError(t, activities.WriteAnonymizedBatch(context.Background(), []anonymizer.AnonymizedRow{{ID: allowedMessageID, Content: "[ИМЯ] [ТЕЛЕФОН]"}}))

	var content string
	var anonymizedAt *time.Time
	err = env.Pool.QueryRow(context.Background(), `select content, anonymized_at from dialogs_messages where id = $1`, allowedMessageID).Scan(&content, &anonymizedAt)
	require.NoError(t, err)
	require.Equal(t, "[ИМЯ] [ТЕЛЕФОН]", content)
	require.NotNil(t, anonymizedAt)

	count, err := activities.CountPendingMessages(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestAnonymizerWriteRechecksOptOutAfterRead(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})
	user := f.CreateUser(TestUserOpts{})
	dialog := f.CreateDialog(user.ID, mode.ID)
	messageID := f.AppendMessage(dialog.ID, "user", "Иван test@example.com")
	_, err := env.Pool.Exec(context.Background(), `update dialogs_messages set created_at = now() - interval '40 days' where id = $1`, messageID)
	require.NoError(t, err)

	activities := anonymizer.NewActivities(env.Pool, anonymizer.Config{MaxBatchBytes: 10 * 1024, MaxBatchRows: 10, MessageAgeDays: 30})
	batch, err := activities.GetMessageBatch(context.Background())
	require.NoError(t, err)
	require.Len(t, batch, 1)

	_, err = env.Pool.Exec(context.Background(), `update users set allow_message_anonymization = false where id = $1`, user.ID)
	require.NoError(t, err)
	require.NoError(t, activities.WriteAnonymizedBatch(context.Background(), []anonymizer.AnonymizedRow{{ID: messageID, Content: "[ИМЯ] [EMAIL]"}}))

	var content string
	var anonymizedAt *time.Time
	err = env.Pool.QueryRow(context.Background(), `select content, anonymized_at from dialogs_messages where id = $1`, messageID).Scan(&content, &anonymizedAt)
	require.NoError(t, err)
	require.Equal(t, "Иван test@example.com", content)
	require.Nil(t, anonymizedAt)
}
