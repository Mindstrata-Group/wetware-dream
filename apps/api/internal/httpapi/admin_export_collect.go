package httpapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func adminExportRoleFilter(raw string) string {
	switch strings.TrimSpace(raw) {
	case "assistant", "user", "all":
		return strings.TrimSpace(raw)
	default:
		return "all"
	}
}

func adminExportRoleWhere(roleFilter string) string {
	switch roleFilter {
	case "assistant":
		return "dm.role = 'assistant'"
	case "user":
		return "dm.role = 'user'"
	default:
		return "dm.role in ('user','assistant')"
	}
}

func adminExportLimit(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return adminExportLimitInt(value)
}

func adminExportLimitInt(value int64) int64 {
	if value <= 0 || value >= 1000001 {
		return 0
	}
	return value
}

func adminExportPromocodeMessageWhere(argPos int) string {
	return fmt.Sprintf(`(
			exists (
				select 1
				from dialog_message_access_usage dmau
				join user_mode_access uma on uma.id = dmau.access_id
				where dmau.dialog_message_id = dm.id
				  and uma.access_type = 'promocode'
				  and uma.source_id = any($%d::bigint[])
			)
			or (
				not exists (
					select 1
					from dialog_message_access_usage dmau_any
					where dmau_any.dialog_message_id = dm.id
				)
				and exists (
					select 1
					from promocode_usages pu
					join promocodes p on p.id = pu.promocode_id
					where pu.user_id = d.user_id
					  and pu.promocode_id = any($%d::bigint[])
					  and dm.created_at >= pu.used_at
					  and (
						p.grants_type in ('all', 'all_modes', 'all_modes_access')
						or (
							p.grants_type in ('mode', 'modes', 'single_mode', 'access_to_mode')
							and (
								coalesce(msg_mode.mode_id, d.mode_id) = p.target_id
								or exists (
									select 1 from promocode_targets pt
									where pt.promocode_id = p.id and pt.target_id = coalesce(msg_mode.mode_id, d.mode_id)
								)
							)
						)
						or (
							p.grants_type in ('tariff', 'tariffs', 'plan')
							and exists (
								select 1
								from tariff_mode tm
								where tm.mode_id = coalesce(msg_mode.mode_id, d.mode_id)
								  and (
									tm.tariff_id = p.target_id
									or exists (
										select 1 from promocode_targets pt
										where pt.promocode_id = p.id and pt.target_id = tm.tariff_id
									)
								  )
							)
						)
						or (
							p.grants_type in ('tariff_group', 'group', 'tariff_groups')
							and exists (
								select 1
								from tariffs t
								join tariff_mode tm on tm.tariff_id = t.id
								where tm.mode_id = coalesce(msg_mode.mode_id, d.mode_id)
								  and (
									t.group_id = p.target_id
									or exists (
										select 1 from promocode_targets pt
										where pt.promocode_id = p.id
										  and pt.target_id = t.group_id
									)
								  )
							)
						)
					  )
				)
			)
		)`, argPos, argPos)
}

func parseAdminExportTime(raw string, endOfDay bool) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			return t.Add(24*time.Hour - time.Nanosecond), true
		}
		return t, true
	}
	return time.Time{}, false
}

func (h Handler) collectAdminExport(ctx context.Context, modeIDs, userIDs, promoIDs []int64, roleFilter string, limit int64, dateFromRaw, dateToRaw string) ([]map[string]any, string, error) {
	where := []string{adminExportRoleWhere(roleFilter)}
	args := []any{}
	if len(modeIDs) > 0 {
		args = append(args, modeIDs)
		where = append(where, fmt.Sprintf("coalesce(msg_mode.mode_id, d.mode_id) = any($%d::bigint[])", len(args)))
	}
	if len(userIDs) > 0 {
		args = append(args, userIDs)
		where = append(where, fmt.Sprintf("d.user_id = any($%d::bigint[])", len(args)))
	}
	if len(promoIDs) > 0 {
		args = append(args, promoIDs)
		where = append(where, adminExportPromocodeMessageWhere(len(args)))
	}
	if dateFrom, ok := parseAdminExportTime(dateFromRaw, false); ok {
		args = append(args, dateFrom)
		where = append(where, fmt.Sprintf("dm.created_at >= $%d", len(args)))
	}
	if dateTo, ok := parseAdminExportTime(dateToRaw, true); ok {
		args = append(args, dateTo)
		where = append(where, fmt.Sprintf("dm.created_at <= $%d", len(args)))
	}
	limitClause := ""
	if limit > 0 {
		args = append(args, limit)
		limitClause = fmt.Sprintf("limit $%d", len(args))
	}
	rows, err := h.DB.Query(ctx, fmt.Sprintf(`
		select d.user_id, coalesce(msg_mode.mode_id, d.mode_id), coalesce(m.name, ''), dm.role, dm.content, dm.created_at
		from dialogs_messages dm
		join users_dialogs d on d.id = dm.dialog_id
		left join lateral (
			select dmau.mode_id
			from dialog_message_access_usage dmau
			where dmau.dialog_message_id = dm.id
			order by dmau.id desc
			limit 1
		) msg_mode on true
		left join modes m on m.id = coalesce(msg_mode.mode_id, d.mode_id)
		where %s
		order by dm.created_at desc
		%s`, strings.Join(where, " and "), limitClause), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []map[string]any{}
	exportText := new(strings.Builder)
	for rows.Next() {
		var uid, mid int64
		var modeName, role, content string
		var createdAt time.Time
		if err := rows.Scan(&uid, &mid, &modeName, &role, &content, &createdAt); err != nil {
			return nil, "", err
		}
		items = append(items, map[string]any{"userId": uid, "modeId": mid, "modeName": modeName, "role": role, "content": content, "createdAt": createdAt})
		_, _ = fmt.Fprintf(exportText, "[%s] user#%d mode#%d %s %s:\n%s\n\n", createdAt.Format(time.RFC3339), uid, mid, modeName, role, content)
	}
	return items, exportText.String(), rows.Err()
}

func (h Handler) adminSummaryModel(ctx context.Context) (string, float64) {
	return h.configuredAIModel(ctx, "ai_summary_model", "ai_summary_temperature", defaultAIFallbackModel, 0.3)
}

func approxTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return (len([]rune(text)) + 3) / 4
}

func nullablePositive(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}
