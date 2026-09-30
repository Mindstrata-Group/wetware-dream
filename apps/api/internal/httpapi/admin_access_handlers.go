package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (h Handler) adminUserAccess(w http.ResponseWriter, r *http.Request, actor AuthenticatedUser, userID int64) {
	switch r.Method {
	case http.MethodGet:
		modes, err := h.getActiveAccessModes(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "activeModes": modes})
	case http.MethodPost:
		var req adminGrantAccessRequest
		if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		if req.ResetLimits {
			modeIDs := append([]int64{}, req.ModeIDs...)
			if req.ModeID > 0 {
				modeIDs = append(modeIDs, req.ModeID)
			}
			updated, err := h.resetAdminAccessLimits(r.Context(), actor.ID, userID, modeIDs)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			h.writeAdminAudit(r.Context(), r, actor.ID, "admin.access.reset_limits", "user", &userID, map[string]any{"modeIds": uniquePositiveIDs(modeIDs), "updated": updated})
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updated, "limitsReset": true})
			return
		}
		granted, extended, err := h.grantAdminAccess(r, actor.ID, userID, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "granted": granted, "extended": extended})
	case http.MethodDelete:
		modeID, _ := strconv.ParseInt(r.URL.Query().Get("modeId"), 10, 64)
		if modeID == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "modeId is required"})
			return
		}
		_, err := h.DB.Exec(r.Context(), `
			update user_mode_access
			set active_to = now(), updated_at = now()
			where user_id = $1 and mode_id = $2 and (active_to is null or active_to > now())`, userID, modeID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := h.resetAdminModeUsage(r.Context(), actor.ID, userID, modeID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = h.DB.Exec(r.Context(), `update users set current_mode = null, current_dialog = null, updated_at = now() where id = $1 and current_mode = $2`, userID, modeID)
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.access.revoke", "user", &userID, map[string]any{"modeId": modeID, "usageReset": true})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "usageReset": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) resetAdminModeUsage(ctx context.Context, actorID, userID, modeID int64) error {
	_, err := h.DB.Exec(ctx, `
		insert into admin_mode_usage_resets (user_id, mode_id, actor_user_id, reset_at, created_at)
		values ($1, $2, $3, now(), now())`, userID, modeID, actorID)
	return err
}

func (h Handler) resetAdminAccessLimits(ctx context.Context, actorID, userID int64, modeIDs []int64) (int64, error) {
	modeIDs = uniquePositiveIDs(modeIDs)
	if len(modeIDs) == 0 {
		return 0, fmt.Errorf("modeIds are required")
	}
	cmd, err := h.DB.Exec(ctx, `
		update user_mode_access
		set daily_message_limit = 0,
		    updated_at = now()
		where user_id = $1
		  and mode_id = any($2::bigint[])
		  and (active_to is null or active_to > now())`, userID, modeIDs)
	if err != nil {
		return 0, err
	}
	for _, modeID := range modeIDs {
		if err := h.resetAdminModeUsage(ctx, actorID, userID, modeID); err != nil {
			return cmd.RowsAffected(), err
		}
	}
	return cmd.RowsAffected(), nil
}

func (h Handler) resolveAdminUserID(ctx context.Context, userID int64, email string) (int64, error) {
	if userID > 0 {
		var id int64
		if err := h.DB.QueryRow(ctx, `select id from users where id = $1 and deleted_at is null`, userID).Scan(&id); err != nil {
			return 0, fmt.Errorf("user not found")
		}
		return id, nil
	}
	email = normalizeEmail(email)
	if email == "" {
		return 0, fmt.Errorf("user id or email is required")
	}
	var id int64
	if err := h.DB.QueryRow(ctx, `select id from users where lower(email) = lower($1) and deleted_at is null`, email).Scan(&id); err != nil {
		return 0, fmt.Errorf("user not found")
	}
	return id, nil
}

func (h Handler) grantAdminAccess(r *http.Request, actorID int64, userID int64, req adminGrantAccessRequest) (int, int, error) {
	modeIDs := append([]int64{}, req.ModeIDs...)
	if req.ModeID > 0 {
		modeIDs = append(modeIDs, req.ModeID)
	}
	if len(modeIDs) == 0 && req.TariffID > 0 {
		rows, err := h.DB.Query(r.Context(), `select tm.mode_id from tariff_mode tm join tariffs t on t.id = tm.tariff_id where tm.tariff_id = $1 and t.archived_at is null order by tm.mode_id`, req.TariffID)
		if err != nil {
			return 0, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return 0, 0, err
			}
			modeIDs = append(modeIDs, id)
		}
	}
	seen := map[int64]struct{}{}
	uniq := make([]int64, 0, len(modeIDs))
	for _, id := range modeIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return 0, 0, fmt.Errorf("at least one mode is required")
	}
	activeTo := time.Now().AddDate(0, 0, 30)
	if req.Days > 0 && req.Days <= 3660 {
		activeTo = time.Now().AddDate(0, 0, req.Days)
	}
	if strings.TrimSpace(req.ActiveTo) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ActiveTo))
		if err != nil {
			return 0, 0, fmt.Errorf("activeTo must be RFC3339")
		}
		activeTo = parsed
	}
	granted := 0
	dailyMessageLimit := normalizedDailyMessageLimit(req.DailyMessageLimit)
	extended := 0
	for _, modeID := range uniq {
		var exists bool
		if err := h.DB.QueryRow(r.Context(), `select exists(select 1 from modes where id = $1)`, modeID).Scan(&exists); err != nil || !exists {
			return 0, 0, fmt.Errorf("mode %d not found", modeID)
		}
		// Lookup existing manual grant from this actor to compute extension semantics.
		// The actual write uses ON CONFLICT so the row is created or extended
		// atomically — no SELECT-then-INSERT race against the unique constraint
		// uq_user_mode_access_source (user_id, mode_id, access_type, source_id).
		var existingActiveTo *time.Time
		var hadExisting bool
		err := h.DB.QueryRow(r.Context(), `
			select active_to
			from user_mode_access
			where user_id = $1 and mode_id = $2 and access_type = 'manual' and source_id = $3`,
			userID, modeID, actorID).Scan(&existingActiveTo)
		if err == nil {
			hadExisting = true
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, err
		}

		base := time.Now()
		if hadExisting && !req.ReplaceActive && existingActiveTo != nil && existingActiveTo.After(base) {
			base = *existingActiveTo
		}
		newActiveTo := base.AddDate(0, 0, 30)
		if req.Days > 0 && req.Days <= 3660 {
			newActiveTo = base.AddDate(0, 0, req.Days)
		}
		if strings.TrimSpace(req.ActiveTo) != "" {
			newActiveTo = activeTo
		}

		if req.ReplaceActive {
			// Expire other active grants for this (user, mode) — except the one
			// we're about to upsert (which keys on access_type='manual' + source_id=actorID).
			if _, err := h.DB.Exec(r.Context(), `
				update user_mode_access
				set active_to = now(), updated_at = now()
				where user_id = $1
				  and mode_id = $2
				  and not (access_type = 'manual' and source_id = $3)
				  and (active_to is null or active_to > now())`, userID, modeID, actorID); err != nil {
				return 0, 0, err
			}
		}

		// Atomic upsert: insert new or extend existing row keyed by
		// (user_id, mode_id, access_type, source_id).
		if _, err := h.DB.Exec(r.Context(), `
			insert into user_mode_access
			    (user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id, created_at, updated_at)
			values
			    ($1, $2, now(), $3, $4, 1000, 'manual', $5, now(), now())
			on conflict on constraint uq_user_mode_access_source do update set
			    active_to = excluded.active_to,
			    daily_message_limit = greatest(coalesce(user_mode_access.daily_message_limit, 0) + excluded.daily_message_limit, excluded.daily_message_limit),
			    priority = greatest(coalesce(user_mode_access.priority, 0), excluded.priority),
			    updated_at = now()`,
			userID, modeID, newActiveTo, dailyMessageLimit, actorID); err != nil {
			return 0, 0, err
		}
		if hadExisting {
			extended++
		} else {
			granted++
		}
	}
	h.writeAdminAudit(r.Context(), r, actorID, "admin.access.grant", "user", &userID, map[string]any{"modeIds": uniq, "activeTo": activeTo})
	return granted, extended, nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func boolValue(v *bool) bool {
	return v != nil && *v
}

func intValue(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}
