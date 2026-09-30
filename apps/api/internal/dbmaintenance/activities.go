package dbmaintenance

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Activities struct {
	pool *pgxpool.Pool
}

type ModesMarkdownResult struct {
	UpdatedRows int64 `json:"updated_rows"`
	SavedChars  int64 `json:"saved_chars"`
}

func NewActivities(pool *pgxpool.Pool) *Activities {
	return &Activities{pool: pool}
}

func (a *Activities) NormalizeModesMarkdown(ctx context.Context) (ModesMarkdownResult, error) {
	var result ModesMarkdownResult
	err := a.pool.QueryRow(ctx, `
		select updated_rows, saved_chars
		from normalize_modes_markdown()`,
	).Scan(&result.UpdatedRows, &result.SavedChars)
	return result, err
}
