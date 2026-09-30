package httpapi

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	promocodeapi "mindstrata-stage1/api/internal/httpapi/promocode"
)

func maxActiveTo(modes []accessModeItem) *time.Time {
	var out *time.Time
	for _, mode := range modes {
		if mode.ActiveTo == nil {
			continue
		}
		if out == nil || mode.ActiveTo.After(*out) {
			t := *mode.ActiveTo
			out = &t
		}
	}
	return out
}

func (h Handler) getActiveAccessModes(ctx context.Context, userID int64) ([]accessModeItem, error) {
	rows, err := h.DB.Query(ctx, `
		select mode_id, name, active_to
		from (
			select distinct on (uma.mode_id) uma.mode_id, m.name, uma.active_to
			from user_mode_access uma
			join modes m on m.id = uma.mode_id
			where uma.user_id = $1
			  and (uma.active_from is null or uma.active_from <= now())
			  and (uma.active_to is null or uma.active_to >= now())
			  and m.hidden_at is null
			order by uma.mode_id, (uma.active_to is null) desc, uma.active_to desc nulls first, uma.id desc
		) access_modes
		order by name asc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []accessModeItem{}
	for rows.Next() {
		var item accessModeItem
		if err := rows.Scan(&item.ModeID, &item.ModeName, &item.ActiveTo); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func getPromocodeForUpdate(ctx context.Context, tx pgx.Tx, code string) (promoRow, error) {
	var p promoRow
	var durationText string

	err := tx.QueryRow(ctx, `
		select id,
		       code,
		       coalesce(max_uses, 0),
		       coalesce(used_count, 0),
		       active_from,
		       active_to,
		       coalesce(duration::text, ''),
		       daily_message_limit,
		       coalesce(access_priority, 0),
		       coalesce(grants_type, ''),
		       coalesce(target_id, 0),
		       first_mode_id
		from promocodes
		where upper(code) = $1
		for update`, strings.ToUpper(code)).Scan(
		&p.ID,
		&p.Code,
		&p.MaxUses,
		&p.UsedCount,
		&p.ActiveFrom,
		&p.ActiveTo,
		&durationText,
		&p.DailyMessageLimit,
		&p.AccessPriority,
		&p.GrantsType,
		&p.TargetID,
		&p.FirstModeID,
	)
	if err != nil {
		return p, err
	}
	p.DurationRaw = durationText
	p.DurationDays = parseDays(durationText)
	p.TargetIDs, err = queryPromocodeTargetIDs(ctx, tx, p.ID, p.TargetID)
	if err != nil {
		return p, err
	}
	return p, nil
}

func parseDays(raw string) int {
	return promocodeapi.ParseDays(raw)
}

func applyDuration(base time.Time, raw string, fallbackDays int) time.Time {
	return promocodeapi.ApplyDuration(base, raw, fallbackDays)
}

func promocodeAlreadyUsedByUser(ctx context.Context, tx pgx.Tx, userID, promocodeID int64) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		select exists(
			select 1
			from promocode_usages
			where user_id = $1 and promocode_id = $2
		)`, userID, promocodeID).Scan(&exists)
	return exists, err
}

func resolvePromocodeModeIDs(ctx context.Context, tx pgx.Tx, p promoRow) ([]int64, error) {
	grantType := strings.ToLower(strings.TrimSpace(p.GrantsType))
	targetIDs := uniquePositiveIDs(p.TargetIDs)
	if len(targetIDs) == 0 && p.TargetID > 0 {
		targetIDs = []int64{p.TargetID}
	}

	switch grantType {
	case "all", "all_modes", "all_modes_access":
		return queryModeIDs(ctx, tx, `select id from modes where hidden_at is null order by id asc`)
	case "mode", "modes", "single_mode", "access_to_mode":
		return targetIDs, nil
	case "tariff", "tariffs", "plan":
		if len(targetIDs) == 0 {
			return []int64{}, nil
		}
		return queryModeIDs(ctx, tx, `select distinct mode_id from tariff_mode where tariff_id = any($1::bigint[]) order by mode_id asc`, targetIDs)
	case "tariff_group", "group", "tariff_groups":
		if len(targetIDs) == 0 {
			return []int64{}, nil
		}
		queries := []string{
			`select distinct tm.mode_id from tariffs t join tariff_mode tm on tm.tariff_id = t.id where t.group_id = any($1::bigint[]) order by tm.mode_id asc`,
			`select distinct tm.mode_id from tariffs t join tariff_mode tm on tm.tariff_id = t.id where t.tariff_group_id = any($1::bigint[]) order by tm.mode_id asc`,
		}
		for _, q := range queries {
			ids, err := queryModeIDs(ctx, tx, q, targetIDs)
			if err != nil {
				continue
			}
			if len(ids) > 0 {
				return ids, nil
			}
		}
		return []int64{}, nil
	default:
		return []int64{}, nil
	}
}

func queryPromocodeTargetIDs(ctx context.Context, tx pgx.Tx, promocodeID, legacyTargetID int64) ([]int64, error) {
	ids, err := queryModeIDs(ctx, tx, `select target_id from promocode_targets where promocode_id = $1 order by target_id asc`, promocodeID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 && legacyTargetID > 0 {
		ids = []int64{legacyTargetID}
	}
	return uniquePositiveIDs(ids), nil
}

func queryModeIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]int64, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func grantPromocodeModeAccesses(ctx context.Context, tx pgx.Tx, userID int64, modeIDs []int64, p promoRow, durationDays int) ([]int64, []int64, error) {
	modeIDs = uniquePositiveIDs(modeIDs)
	if len(modeIDs) == 0 {
		return []int64{}, []int64{}, nil
	}
	activeTo := applyDuration(time.Now(), p.DurationRaw, durationDays)
	dailyMessageLimit := normalizedDailyMessageLimit(p.DailyMessageLimit)
	rows, err := tx.Query(ctx, `
		with requested as (
			select distinct unnest($2::bigint[]) as mode_id
		), existing as (
			select distinct uma.mode_id
			from user_mode_access uma
			join requested r on r.mode_id = uma.mode_id
			where uma.user_id = $1
			  and (uma.active_from is null or uma.active_from <= now())
			  and (uma.active_to is null or uma.active_to >= now())
		), inserted as (
			insert into user_mode_access
				(user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id, created_at, updated_at)
			select $1, r.mode_id, now(), $3, $4, $5, 'promocode', $6, now(), now()
			from requested r
			returning mode_id
		)
		select i.mode_id, exists(select 1 from existing e where e.mode_id = i.mode_id) as extended
		from inserted i
		order by i.mode_id asc`, userID, modeIDs, activeTo, dailyMessageLimit, p.AccessPriority, p.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	granted := []int64{}
	extended := []int64{}
	for rows.Next() {
		var modeID int64
		var wasExtended bool
		if err := rows.Scan(&modeID, &wasExtended); err != nil {
			return nil, nil, err
		}
		if wasExtended {
			extended = append(extended, modeID)
		} else {
			granted = append(granted, modeID)
		}
	}
	return granted, extended, rows.Err()
}

func mergeAccessModeLists(groups ...[]accessModeItem) []accessModeItem {
	seen := map[int64]struct{}{}
	out := []accessModeItem{}
	for _, group := range groups {
		for _, mode := range group {
			if _, ok := seen[mode.ModeID]; ok {
				continue
			}
			seen[mode.ModeID] = struct{}{}
			out = append(out, mode)
		}
	}
	return out
}

func splitAccessModesByIDs(modes []accessModeItem, grantedIDs, extendedIDs []int64) ([]accessModeItem, []accessModeItem) {
	grantedSet := map[int64]struct{}{}
	for _, id := range grantedIDs {
		grantedSet[id] = struct{}{}
	}
	extendedSet := map[int64]struct{}{}
	for _, id := range extendedIDs {
		extendedSet[id] = struct{}{}
	}
	granted := []accessModeItem{}
	extended := []accessModeItem{}
	for _, mode := range modes {
		if _, ok := grantedSet[mode.ModeID]; ok {
			mode.AccessAction = "granted"
			granted = append(granted, mode)
		}
		if _, ok := extendedSet[mode.ModeID]; ok {
			mode.AccessAction = "extended"
			extended = append(extended, mode)
		}
	}
	return granted, extended
}

func reorderFirstMode(modes []accessModeItem, firstModeID int64) []accessModeItem {
	idx := -1
	for i, m := range modes {
		if m.ModeID == firstModeID {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return modes
	}
	out := make([]accessModeItem, 0, len(modes))
	out = append(out, modes[idx])
	out = append(out, modes[:idx]...)
	out = append(out, modes[idx+1:]...)
	return out
}

// K-NEW5-2: grantAdminRole must not demote an owner.
// Attack: an admin creates a promocode with grantsType='admin_role' and gets the owner
// to apply it (phishing); without this check the owner would be demoted to admin.
// Also: do not upgrade again if the user is already admin/owner (no-op).
func grantAdminRole(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx, `
		update users
		set role = 'admin',
		    status = 'active',
		    updated_at = now()
		where id = $1
		  and deleted_at is null
		  and role not in ('owner', 'admin')`, userID)
	return err
}
