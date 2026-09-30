package httpapi

import (
	"context"
	"strings"
)

func (h Handler) adminPromoModeIndex(ctx context.Context) ([]int64, map[int64][]int64, map[int64][]int64, error) {
	allModeIDs, err := h.adminExportModeIDs(ctx, `select id from modes where hidden_at is null order by id asc`)
	if err != nil {
		return nil, nil, nil, err
	}
	tariffModeIDs, err := h.adminExportGroupedModeIDs(ctx, `select tariff_id, mode_id from tariff_mode order by tariff_id asc, mode_id asc`)
	if err != nil {
		return nil, nil, nil, err
	}
	groupModeIDs, err := h.adminExportGroupedModeIDs(ctx, `
		select t.group_id, tm.mode_id
		from tariffs t
		join tariff_mode tm on tm.tariff_id = t.id
		where t.group_id is not null
		order by t.group_id asc, tm.mode_id asc`)
	if err != nil {
		return nil, nil, nil, err
	}
	return allModeIDs, tariffModeIDs, groupModeIDs, nil
}

func (h Handler) adminExportModeIDs(ctx context.Context, sql string, args ...any) ([]int64, error) {
	rows, err := h.DB.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (h Handler) adminExportGroupedModeIDs(ctx context.Context, sql string, args ...any) (map[int64][]int64, error) {
	rows, err := h.DB.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]int64{}
	seen := map[int64]map[int64]struct{}{}
	for rows.Next() {
		var groupID, modeID int64
		if err := rows.Scan(&groupID, &modeID); err != nil {
			return nil, err
		}
		if seen[groupID] == nil {
			seen[groupID] = map[int64]struct{}{}
		}
		if _, ok := seen[groupID][modeID]; ok {
			continue
		}
		seen[groupID][modeID] = struct{}{}
		out[groupID] = append(out[groupID], modeID)
	}
	return out, rows.Err()
}

func adminPromoModeIDsFromIndex(grantsType string, targetID int64, allModeIDs []int64, tariffModeIDs, groupModeIDs map[int64][]int64) []int64 {
	switch strings.TrimSpace(grantsType) {
	case "all", "all_modes", "all_modes_access":
		return append([]int64(nil), allModeIDs...)
	case "mode", "modes", "single_mode", "access_to_mode":
		if targetID > 0 {
			return []int64{targetID}
		}
	case "tariff", "tariffs", "plan":
		return append([]int64(nil), tariffModeIDs[targetID]...)
	case "tariff_group", "group", "tariff_groups":
		return append([]int64(nil), groupModeIDs[targetID]...)
	}
	if targetID > 0 {
		return []int64{targetID}
	}
	return []int64{}
}

func (h Handler) promocodeTargetIDs(ctx context.Context, promocodeID, legacyTargetID int64) ([]int64, error) {
	rows, err := h.DB.Query(ctx, `select target_id from promocode_targets where promocode_id = $1 order by target_id asc`, promocodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 && legacyTargetID > 0 {
		out = append(out, legacyTargetID)
	}
	return uniquePositiveIDs(out), nil
}

func (h Handler) promocodeModeIDs(ctx context.Context, grantsType string, targetID int64) ([]int64, error) {
	targetIDs := []int64{}
	if targetID > 0 {
		targetIDs = append(targetIDs, targetID)
	}
	return h.promocodeModeIDsForTargets(ctx, grantsType, targetIDs)
}

func (h Handler) promocodeModeIDsForTargets(ctx context.Context, grantsType string, targetIDs []int64) ([]int64, error) {
	targetIDs = uniquePositiveIDs(targetIDs)
	switch strings.TrimSpace(grantsType) {
	case "all", "all_modes", "all_modes_access":
		rows, err := h.DB.Query(ctx, `select id from modes where hidden_at is null order by id asc`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []int64{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	case "mode", "modes", "single_mode", "access_to_mode":
		return targetIDs, nil
	case "tariff", "tariffs", "plan":
		if len(targetIDs) == 0 {
			return []int64{}, nil
		}
		rows, err := h.DB.Query(ctx, `select distinct mode_id from tariff_mode where tariff_id = any($1::bigint[]) order by mode_id asc`, targetIDs)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []int64{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	case "tariff_group", "group", "tariff_groups":
		if len(targetIDs) == 0 {
			return []int64{}, nil
		}
		rows, err := h.DB.Query(ctx, `select distinct tm.mode_id from tariffs t join tariff_mode tm on tm.tariff_id = t.id where t.group_id = any($1::bigint[]) order by tm.mode_id asc`, targetIDs)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []int64{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	default:
		return []int64{}, nil
	}
}
