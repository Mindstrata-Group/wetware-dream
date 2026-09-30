package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// decrementDailyMessageCount rolls back delta quota slots.
// K-2: called on an AI error after tryIncrementDailyMessageCount.
// Uses greatest(0, ...) so it never goes below zero.
func (h Handler) decrementDailyMessageCount(ctx context.Context, userID int64, delta int64) {
	if delta <= 0 || h.DB == nil {
		return
	}
	_, _ = h.DB.Exec(ctx, `
		update daily_message_counts
		set count = greatest(0, count - $2::int), updated_at = now()
		where user_id = $1 and date = current_date`, userID, delta)
}

func (h Handler) incrementDailyMessageCount(ctx context.Context, userID int64, delta int64) (int64, error) {
	if delta == 0 {
		return 0, nil
	}
	var newCount int64
	err := h.DB.QueryRow(ctx, `
		insert into daily_message_counts (user_id, date, count, updated_at)
		values ($1, current_date, $2::int, now())
		on conflict (user_id, date)
		do update set
			count = daily_message_counts.count + excluded.count,
			updated_at = now()
		returning count`, userID, delta).Scan(&newCount)
	if err == nil {
		metricDailyCounterIncrOps.Add(1)
	}
	return newCount, err
}

// tryIncrementDailyMessageCount atomically checks-and-increments the daily
// counter for userID. It returns (newCount, true, nil) if the increment stayed
// within `limit`, or (0, false, nil) if the limit would be exceeded.
//
// Atomicity guarantee:
//
//   - First call of the day (no row): INSERT with count=delta. Since we
//     guard `delta > limit` up front, this respects the limit.
//   - Subsequent calls: ON CONFLICT DO UPDATE acquires a row lock, then
//     re-evaluates `count + delta <= limit` AFTER the lock is held. Two
//     concurrent callers cannot both pass: the second waits, re-reads the
//     post-lock count, and is filtered out by the WHERE → RETURNING returns
//     no rows → ErrNoRows → false.
//
// Note: a CTE with `INSERT ... DO NOTHING` followed by `UPDATE` in the same
// statement does NOT work for this pattern because PG's snapshot isolation
// hides the CTE's INSERT from the UPDATE in the main query. INSERT ... ON
// CONFLICT DO UPDATE is the only single-statement pattern that correctly
// handles "create row if missing, else atomically increment with limit".
//
// This replaces the previous check-then-act pattern (dailyQuota → if
// Remaining>0 → insert → increment) which had a race window: N concurrent
// requests could all pass the check and all insert, exceeding the limit.
//
// Caller passes `limit` (typically from dailyQuota's Limit field).
func (h Handler) tryIncrementDailyMessageCount(ctx context.Context, userID, limit, delta int64) (int64, bool, error) {
	if delta <= 0 {
		return 0, false, errors.New("tryIncrementDailyMessageCount: delta must be > 0")
	}
	if limit <= 0 || delta > limit {
		return 0, false, nil
	}
	var newCount int64
	err := h.DB.QueryRow(ctx, `
		insert into daily_message_counts (user_id, date, count, updated_at)
		values ($1, current_date, $2::int, now())
		on conflict (user_id, date)
		do update set
			count = daily_message_counts.count + excluded.count,
			updated_at = now()
		where daily_message_counts.count + excluded.count <= $3::int
		returning count`, userID, delta, limit).Scan(&newCount)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT WHERE filtered the row → adding delta would exceed limit.
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	metricDailyCounterIncrOps.Add(1)
	return newCount, true, nil
}
