package anonymizer

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// The DB computes the running content size and returns messages until the
// total octet_length exceeds MaxBatchBytes. The first message is always
// included, even if it alone is larger than the limit.
const getMessageBatchSQL = `
	SELECT id, content FROM (
		SELECT dm.id, dm.content,
			SUM(octet_length(dm.content)) OVER (
				ORDER BY dm.id
				ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
			) AS running_bytes,
			ROW_NUMBER() OVER (ORDER BY dm.id) AS rn
		FROM dialogs_messages dm
		JOIN users_dialogs ud ON ud.id = dm.dialog_id
		JOIN users u ON u.id = ud.user_id
		WHERE dm.anonymized_at IS NULL
		  AND dm.created_at < NOW() - ($1 * INTERVAL '1 day')
		  AND u.allow_message_anonymization = TRUE
		  AND u.deleted_at IS NULL
		ORDER BY dm.id
		LIMIT $2
	) sub
	WHERE running_bytes <= $3 OR rn = 1
	ORDER BY id`

const writeAnonymizedBatchSQL = `
	UPDATE dialogs_messages dm
	SET content = $1, anonymized_at = NOW()
	FROM users_dialogs ud
	JOIN users u ON u.id = ud.user_id
	WHERE dm.id = $2
	  AND ud.id = dm.dialog_id
	  AND dm.anonymized_at IS NULL
	  AND u.allow_message_anonymization = TRUE
	  AND u.deleted_at IS NULL`

const countPendingMessagesSQL = `
	SELECT COUNT(*)
	FROM dialogs_messages dm
	JOIN users_dialogs ud ON ud.id = dm.dialog_id
	JOIN users u ON u.id = ud.user_id
	WHERE dm.anonymized_at IS NULL
	  AND dm.created_at < NOW() - ($1 * INTERVAL '1 day')
	  AND u.allow_message_anonymization = TRUE
	  AND u.deleted_at IS NULL`

type dbConn interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Activities struct {
	db  dbConn
	cfg Config
}

func NewActivities(db dbConn, cfg Config) *Activities {
	return &Activities{db: db, cfg: cfg}
}

type MessageRow struct {
	ID      int64
	Content string
}

// AnonymizeResult is the Python activity's reply: anonymised texts + replacement statistics.
type AnonymizeResult struct {
	Texts []string       `json:"texts"`
	Stats map[string]int `json:"stats"`
}

// WorkflowResult is the outcome of the whole run: shown in the Temporal UI in the Result field.
type WorkflowResult struct {
	Pending   int64          `json:"pending"`
	Processed int            `json:"processed"`
	Stats     map[string]int `json:"stats"`
}

func (a *Activities) GetMessageBatch(ctx context.Context) ([]MessageRow, error) {
	rows, err := a.db.Query(ctx, getMessageBatchSQL, a.cfg.MessageAgeDays, a.cfg.MaxBatchRows, a.cfg.MaxBatchBytes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	batch := make([]MessageRow, 0, a.cfg.MaxBatchRows)
	for rows.Next() {
		var row MessageRow
		if err := rows.Scan(&row.ID, &row.Content); err != nil {
			return nil, err
		}
		batch = append(batch, row)
	}
	return batch, rows.Err()
}

type AnonymizedRow struct {
	ID      int64
	Content string
}

func (a *Activities) WriteAnonymizedBatch(ctx context.Context, rows []AnonymizedRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, row := range rows {
		if _, err := tx.Exec(ctx, writeAnonymizedBatchSQL, row.Content, row.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (a *Activities) CountPendingMessages(ctx context.Context) (int64, error) {
	var total int64
	err := a.db.QueryRow(ctx, countPendingMessagesSQL, a.cfg.MessageAgeDays).Scan(&total)
	return total, err
}
