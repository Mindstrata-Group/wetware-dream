package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	promocodeUpdateScopeNewOnly        = "new_only"
	promocodeUpdateScopeAllActivations = "all_activations"
)

var errAdminPromocodeBadRequest = errors.New("bad promocode update request")

type adminPromocodePatchRequest struct {
	ActiveTo    string  `json:"activeTo"`
	MaxUses     *int    `json:"maxUses"`
	GrantsType  string  `json:"grantsType"`
	TargetIDs   []int64 `json:"targetIds"`
	FirstModeID *int64  `json:"firstModeId"`
	UpdateScope string  `json:"updateScope"`
}

type adminPromocodeUpdateResult struct {
	ActiveTo            *time.Time
	MaxUses             int64
	UpdateScope         string
	TargetsUpdated      bool
	OnlyActivation      bool
	UpsertedAccesses    int64
	DeactivatedAccesses int64
	RoleUpgrades        int64
}

func adminPromocodeUpdateStatus(err error) int {
	if errors.Is(err, errAdminPromocodeBadRequest) || errors.Is(err, pgx.ErrNoRows) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func badPromocodeUpdate(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errAdminPromocodeBadRequest, fmt.Sprintf(format, args...))
}

func (h Handler) updateAdminPromocode(r *http.Request, id int64, req adminPromocodePatchRequest) (adminPromocodeUpdateResult, error) {
	ctx := r.Context()
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return adminPromocodeUpdateResult{}, err
	}
	defer tx.Rollback(ctx)

	current, err := loadAdminPromocodeForUpdate(ctx, tx, id)
	if err != nil {
		return adminPromocodeUpdateResult{}, err
	}

	activeToChanged := strings.TrimSpace(req.ActiveTo) != ""
	var activeTo *time.Time
	if activeToChanged {
		activeTo, err = parseOptionalAdminEndTime(req.ActiveTo)
		if err != nil || activeTo == nil || !activeTo.After(time.Now()) {
			return adminPromocodeUpdateResult{}, badPromocodeUpdate("activeTo must be later than now")
		}
	}

	maxUsesChanged := req.MaxUses != nil
	if maxUsesChanged && *req.MaxUses < 0 {
		return adminPromocodeUpdateResult{}, badPromocodeUpdate("maxUses must be non-negative")
	}
	maxUses := current.MaxUses
	if maxUsesChanged {
		maxUses = maxInt64(current.MaxUses, current.UsedCount+1, int64(*req.MaxUses))
	}

	targetsUpdated := strings.TrimSpace(req.GrantsType) != "" || req.TargetIDs != nil || req.FirstModeID != nil
	grantType := current.GrantsType
	targetIDs := append([]int64(nil), current.TargetIDs...)
	firstModeID := current.FirstModeID
	legacyTargetID := current.TargetID
	modeIDs := []int64{}
	if targetsUpdated {
		if raw := strings.TrimSpace(req.GrantsType); raw != "" {
			grantType, err = canonicalAdminPromocodeGrantType(raw)
			if err != nil {
				return adminPromocodeUpdateResult{}, err
			}
		}
		if req.TargetIDs != nil {
			targetIDs = uniquePositiveIDs(req.TargetIDs)
		}
		if grantType == "admin_role" {
			targetIDs = []int64{}
			firstModeID = nil
		} else {
			if len(targetIDs) == 0 {
				return adminPromocodeUpdateResult{}, badPromocodeUpdate("select at least one promocode target")
			}
			if err := validateAdminPromocodeTargets(ctx, tx, grantType, targetIDs); err != nil {
				return adminPromocodeUpdateResult{}, err
			}
			firstModeID, err = normalizePromocodeFirstModeID(grantType, targetIDs, req.FirstModeID, firstModeID)
			if err != nil {
				return adminPromocodeUpdateResult{}, err
			}
			modeIDs, err = resolvePromocodeModeIDs(ctx, tx, promoRow{ID: id, GrantsType: grantType, TargetID: targetIDs[0], TargetIDs: targetIDs})
			if err != nil {
				return adminPromocodeUpdateResult{}, err
			}
			if len(modeIDs) == 0 {
				return adminPromocodeUpdateResult{}, badPromocodeUpdate("promocode target has no available modes")
			}
		}
		legacyTargetID = int64(0)
		if len(targetIDs) > 0 {
			legacyTargetID = targetIDs[0]
		}
	}

	_, err = tx.Exec(ctx, `
		update promocodes
		set active_to = case when $2 then $3 else active_to end,
		    active_from = case when ($2 or $4) then coalesce(active_from, now()) else active_from end,
		    max_uses = case when $4 then greatest(coalesce(max_uses, 0), coalesce(used_count, 0) + 1, $5) else max_uses end,
		    grants_type = case when $6 then $7 else grants_type end,
		    target_id = case when $6 then $8 else target_id end,
		    first_mode_id = case when $6 then $9 else first_mode_id end,
		    updated_at = now()
		where id = $1`,
		id, activeToChanged, activeTo, maxUsesChanged, maxUses, targetsUpdated, grantType, legacyTargetID, firstModeID)
	if err != nil {
		return adminPromocodeUpdateResult{}, err
	}

	if targetsUpdated {
		if _, err := tx.Exec(ctx, `delete from promocode_targets where promocode_id = $1`, id); err != nil {
			return adminPromocodeUpdateResult{}, err
		}
		if len(targetIDs) > 0 {
			if _, err := tx.Exec(ctx, `
				insert into promocode_targets (promocode_id, target_id, created_at)
				select $1, unnest($2::bigint[]), now()
				on conflict do nothing`, id, targetIDs); err != nil {
				return adminPromocodeUpdateResult{}, err
			}
		}
	}

	result := adminPromocodeUpdateResult{
		ActiveTo:       activeTo,
		MaxUses:        maxUses,
		UpdateScope:    normalizePromocodeUpdateScope(req.UpdateScope),
		TargetsUpdated: targetsUpdated,
		OnlyActivation: !targetsUpdated,
	}
	if targetsUpdated && result.UpdateScope == promocodeUpdateScopeAllActivations {
		updatedPromo := current
		updatedPromo.GrantsType = grantType
		updatedPromo.TargetID = legacyTargetID
		updatedPromo.TargetIDs = targetIDs
		updatedPromo.FirstModeID = firstModeID
		syncResult, err := syncPromocodeActivations(ctx, tx, updatedPromo, modeIDs)
		if err != nil {
			return adminPromocodeUpdateResult{}, err
		}
		result.UpsertedAccesses = syncResult.UpsertedAccesses
		result.DeactivatedAccesses = syncResult.DeactivatedAccesses
		result.RoleUpgrades = syncResult.RoleUpgrades
	}

	if err := tx.Commit(ctx); err != nil {
		return adminPromocodeUpdateResult{}, err
	}
	return result, nil
}

func loadAdminPromocodeForUpdate(ctx context.Context, tx pgx.Tx, id int64) (promoRow, error) {
	var p promoRow
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
		where id = $1
		for update`, id).Scan(
		&p.ID,
		&p.Code,
		&p.MaxUses,
		&p.UsedCount,
		&p.ActiveFrom,
		&p.ActiveTo,
		&p.DurationRaw,
		&p.DailyMessageLimit,
		&p.AccessPriority,
		&p.GrantsType,
		&p.TargetID,
		&p.FirstModeID,
	)
	if err != nil {
		return p, err
	}
	p.DurationDays = parseDays(p.DurationRaw)
	p.TargetIDs, err = queryPromocodeTargetIDs(ctx, tx, p.ID, p.TargetID)
	return p, err
}

func canonicalAdminPromocodeGrantType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "mode", "modes", "single_mode", "access_to_mode":
		return "mode", nil
	case "tariff", "tariffs", "plan":
		return "tariff", nil
	case "admin_role":
		return "admin_role", nil
	default:
		return "", badPromocodeUpdate("unsupported promocode grant type")
	}
}

func validateAdminPromocodeTargets(ctx context.Context, tx pgx.Tx, grantType string, targetIDs []int64) error {
	if len(targetIDs) == 0 {
		return badPromocodeUpdate("select at least one promocode target")
	}
	var count int
	var err error
	switch grantType {
	case "mode":
		err = tx.QueryRow(ctx, `select count(distinct id) from modes where hidden_at is null and id = any($1::bigint[])`, targetIDs).Scan(&count)
	case "tariff":
		err = tx.QueryRow(ctx, `select count(distinct id) from tariffs where archived_at is null and id = any($1::bigint[])`, targetIDs).Scan(&count)
	default:
		return badPromocodeUpdate("unsupported promocode grant type")
	}
	if err != nil {
		return err
	}
	if count != len(targetIDs) {
		return badPromocodeUpdate("one or more promocode targets are inactive or not found")
	}
	return nil
}

func normalizePromocodeFirstModeID(grantType string, targetIDs []int64, requested, fallback *int64) (*int64, error) {
	if grantType != "mode" {
		return nil, nil
	}
	if requested != nil {
		if *requested <= 0 {
			return nil, nil
		}
		for _, id := range targetIDs {
			if id == *requested {
				value := *requested
				return &value, nil
			}
		}
		return nil, badPromocodeUpdate("firstModeId must be one of targetIds")
	}
	if fallback != nil && *fallback > 0 {
		for _, id := range targetIDs {
			if id == *fallback {
				value := *fallback
				return &value, nil
			}
		}
	}
	return nil, nil
}

func normalizePromocodeUpdateScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case promocodeUpdateScopeAllActivations:
		return promocodeUpdateScopeAllActivations
	default:
		return promocodeUpdateScopeNewOnly
	}
}

func maxInt64(values ...int64) int64 {
	out := values[0]
	for _, value := range values[1:] {
		if value > out {
			out = value
		}
	}
	return out
}

type promocodeActivationSyncResult struct {
	UpsertedAccesses    int64
	DeactivatedAccesses int64
	RoleUpgrades        int64
}

func syncPromocodeActivations(ctx context.Context, tx pgx.Tx, promo promoRow, modeIDs []int64) (promocodeActivationSyncResult, error) {
	modeIDs = uniquePositiveIDs(modeIDs)
	result := promocodeActivationSyncResult{}
	if strings.TrimSpace(promo.GrantsType) == "admin_role" {
		cmd, err := tx.Exec(ctx, `
			update users
			set role = 'admin', updated_at = now()
			where id in (select user_id from promocode_usages where promocode_id = $1)
			  and role not in ('owner', 'admin')`, promo.ID)
		if err != nil {
			return result, err
		}
		result.RoleUpgrades = cmd.RowsAffected()
	} else if len(modeIDs) > 0 {
		cmd, err := tx.Exec(ctx, `
			with desired as (
				select distinct unnest($2::bigint[]) as mode_id
			), activations as (
				select user_id, used_at
				from promocode_usages
				where promocode_id = $1
			), promo as (
				select id, duration, daily_message_limit, access_priority
				from promocodes
				where id = $1
			)
			insert into user_mode_access
				(user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id, created_at, updated_at)
			select a.user_id,
			       d.mode_id,
			       a.used_at,
			       a.used_at + p.duration,
			       p.daily_message_limit,
			       p.access_priority,
			       'promocode',
			       p.id,
			       now(),
			       now()
			from activations a
			cross join desired d
			cross join promo p
			where a.used_at + p.duration > now()
			on conflict (user_id, mode_id, access_type, source_id) do update
			set active_from = least(user_mode_access.active_from, excluded.active_from),
			    active_to = excluded.active_to,
			    daily_message_limit = excluded.daily_message_limit,
			    priority = excluded.priority,
			    updated_at = now()`, promo.ID, modeIDs)
		if err != nil {
			return result, err
		}
		result.UpsertedAccesses = cmd.RowsAffected()
	}

	cmd, err := tx.Exec(ctx, `
		update user_mode_access
		set active_to = least(active_to, now()), updated_at = now()
		where access_type = 'promocode'
		  and source_id = $1
		  and active_to > now()
		  and not (mode_id = any($2::bigint[]))`, promo.ID, modeIDs)
	if err != nil {
		return result, err
	}
	result.DeactivatedAccesses = cmd.RowsAffected()
	return result, nil
}
