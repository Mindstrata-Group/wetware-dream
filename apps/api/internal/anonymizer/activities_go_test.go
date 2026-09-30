package anonymizer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestActivitySQLGuardsOptOutAndDeletedUsers(t *testing.T) {
	t.Parallel()

	for name, sql := range map[string]string{
		"get batch": getMessageBatchSQL,
		"count":     countPendingMessagesSQL,
		"write":     writeAnonymizedBatchSQL,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			normalized := strings.ToLower(sql)
			require.Contains(t, normalized, "allow_message_anonymization = true")
			require.Contains(t, normalized, "u.deleted_at is null")
		})
	}
}

func TestWriteSQLRechecksPendingRowAndOwnership(t *testing.T) {
	t.Parallel()

	normalized := strings.ToLower(writeAnonymizedBatchSQL)
	require.Contains(t, normalized, "update dialogs_messages dm")
	require.Contains(t, normalized, "from users_dialogs ud")
	require.Contains(t, normalized, "join users u")
	require.Contains(t, normalized, "ud.id = dm.dialog_id")
	require.Contains(t, normalized, "dm.anonymized_at is null")
}

func TestGetMessageBatchSQLUsesRunningBytesWindow(t *testing.T) {
	t.Parallel()

	normalized := strings.ToLower(getMessageBatchSQL)
	require.Contains(t, normalized, "octet_length")
	require.Contains(t, normalized, "running_bytes")
	require.Contains(t, normalized, "rn = 1") // the first message is always included
}

func TestGetMessageBatchScansRows(t *testing.T) {
	t.Parallel()

	rows := &fakeRows{values: [][]any{{int64(11), "hello"}, {int64(12), "world"}}}
	db := &fakeDB{rows: rows}
	activities := NewActivities(db, Config{MaxBatchBytes: 100, MaxBatchRows: 50, MessageAgeDays: 30})

	batch, err := activities.GetMessageBatch(context.Background())

	require.NoError(t, err)
	require.Equal(t, []MessageRow{{ID: 11, Content: "hello"}, {ID: 12, Content: "world"}}, batch)
	require.Equal(t, getMessageBatchSQL, db.querySQL)
	require.Equal(t, []any{30, 50, 100}, db.queryArgs)
	require.True(t, rows.closed)
}

func TestCountPendingMessagesScansCount(t *testing.T) {
	t.Parallel()

	db := &fakeDB{row: fakeRow{values: []any{int64(42)}}}
	activities := NewActivities(db, Config{MessageAgeDays: 45})

	count, err := activities.CountPendingMessages(context.Background())

	require.NoError(t, err)
	require.Equal(t, int64(42), count)
	require.Equal(t, countPendingMessagesSQL, db.queryRowSQL)
	require.Equal(t, []any{45}, db.queryRowArgs)
}

func TestWriteAnonymizedBatchUsesTransaction(t *testing.T) {
	t.Parallel()

	tx := &fakeTx{}
	db := &fakeDB{tx: tx}
	activities := NewActivities(db, Config{})

	err := activities.WriteAnonymizedBatch(context.Background(), []AnonymizedRow{{ID: 7, Content: "[EMAIL]"}, {ID: 8, Content: "[ИМЯ]"}})

	require.NoError(t, err)
	require.True(t, tx.committed)
	require.True(t, tx.rolledBack, "deferred rollback stays safe after commit")
	require.Equal(t, []execCall{
		{sql: writeAnonymizedBatchSQL, args: []any{"[EMAIL]", int64(7)}},
		{sql: writeAnonymizedBatchSQL, args: []any{"[ИМЯ]", int64(8)}},
	}, tx.execs)
}

func TestWriteAnonymizedBatchSkipsTransactionForEmptyRows(t *testing.T) {
	t.Parallel()

	db := &fakeDB{}
	activities := NewActivities(db, Config{})

	require.NoError(t, activities.WriteAnonymizedBatch(context.Background(), nil))
	require.False(t, db.beginCalled)
}

func TestWriteAnonymizedBatchRollsBackOnError(t *testing.T) {
	t.Parallel()

	execErr := errors.New("update failed")
	tx := &fakeTx{execErr: execErr}
	db := &fakeDB{tx: tx}
	activities := NewActivities(db, Config{})

	err := activities.WriteAnonymizedBatch(context.Background(), []AnonymizedRow{{ID: 7, Content: "[EMAIL]"}})

	require.ErrorIs(t, err, execErr)
	require.False(t, tx.committed)
	require.True(t, tx.rolledBack)
}

func TestGetMessageBatchReturnsQueryError(t *testing.T) {
	t.Parallel()

	queryErr := errors.New("query failed")
	activities := NewActivities(&fakeDB{queryErr: queryErr}, Config{MaxBatchBytes: 100, MaxBatchRows: 10, MessageAgeDays: 30})

	_, err := activities.GetMessageBatch(context.Background())
	require.ErrorIs(t, err, queryErr)
}

func TestCountPendingMessagesReturnsScanError(t *testing.T) {
	t.Parallel()

	scanErr := errors.New("scan failed")
	activities := NewActivities(&fakeDB{row: fakeRow{err: scanErr}}, Config{MessageAgeDays: 30})

	_, err := activities.CountPendingMessages(context.Background())
	require.ErrorIs(t, err, scanErr)
}

func TestWriteAnonymizedBatchReturnsBeginAndCommitErrors(t *testing.T) {
	t.Parallel()

	beginErr := errors.New("begin failed")
	activities := NewActivities(&fakeDB{beginErr: beginErr}, Config{})
	err := activities.WriteAnonymizedBatch(context.Background(), []AnonymizedRow{{ID: 1, Content: "x"}})
	require.ErrorIs(t, err, beginErr)

	commitErr := errors.New("commit failed")
	activities = NewActivities(&fakeDB{tx: &fakeTx{commitErr: commitErr}}, Config{})
	err = activities.WriteAnonymizedBatch(context.Background(), []AnonymizedRow{{ID: 1, Content: "x"}})
	require.ErrorIs(t, err, commitErr)
}

type fakeDB struct {
	rows pgx.Rows
	row  pgx.Row
	tx   pgx.Tx

	queryErr error
	beginErr error

	querySQL     string
	queryArgs    []any
	queryRowSQL  string
	queryRowArgs []any
	beginCalled  bool
}

func (db *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.querySQL = sql
	db.queryArgs = append([]any(nil), args...)
	return db.rows, db.queryErr
}

func (db *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	db.queryRowSQL = sql
	db.queryRowArgs = append([]any(nil), args...)
	return db.row
}

func (db *fakeDB) Begin(context.Context) (pgx.Tx, error) {
	db.beginCalled = true
	return db.tx, db.beginErr
}

type fakeRows struct {
	values [][]any
	idx    int
	closed bool
}

func (r *fakeRows) Close()                                       { r.closed = true }
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Values() ([]any, error)                       { return r.values[r.idx-1], nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }
func (r *fakeRows) Next() bool {
	if r.idx >= len(r.values) {
		r.closed = true
		return false
	}
	r.idx++
	return true
}
func (r *fakeRows) Scan(dest ...any) error {
	values := r.values[r.idx-1]
	for i := range dest {
		switch target := dest[i].(type) {
		case *int64:
			*target = values[i].(int64)
		case *string:
			*target = values[i].(string)
		}
	}
	return nil
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch target := dest[i].(type) {
		case *int64:
			*target = r.values[i].(int64)
		}
	}
	return nil
}

type execCall struct {
	sql  string
	args []any
}

type fakeTx struct {
	execs      []execCall
	execErr    error
	commitErr  error
	committed  bool
	rolledBack bool
}

func (tx *fakeTx) Begin(context.Context) (pgx.Tx, error) { return tx, nil }
func (tx *fakeTx) Commit(context.Context) error {
	tx.committed = true
	return tx.commitErr
}
func (tx *fakeTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}
func (tx *fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (tx *fakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (tx *fakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (tx *fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (tx *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.execs = append(tx.execs, execCall{sql: sql, args: append([]any(nil), args...)})
	return pgconn.CommandTag{}, tx.execErr
}
func (tx *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (tx *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row        { return fakeRow{} }
func (tx *fakeTx) Conn() *pgx.Conn                                         { return nil }
func (tx *fakeTx) TypeMap() *pgtype.Map                                    { return nil }
