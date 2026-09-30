package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type accessModeItem struct {
	ModeID       int64      `json:"modeId"`
	ModeName     string     `json:"modeName"`
	ActiveTo     *time.Time `json:"activeTo,omitempty"`
	AccessAction string     `json:"accessAction,omitempty"`
}

type accessStatusResponse struct {
	OK          bool             `json:"ok"`
	UserID      int64            `json:"userId"`
	HasAccess   bool             `json:"hasAccess"`
	ActiveModes []accessModeItem `json:"activeModes"`
	ActiveTo    *time.Time       `json:"activeTo,omitempty"`
}

type applyPromocodeRequest struct {
	Code string `json:"code"`
}

type applyPromocodeResponse struct {
	OK            bool             `json:"ok"`
	Code          string           `json:"code,omitempty"`
	AccessDays    int              `json:"accessDays,omitempty"`
	Modes         []accessModeItem `json:"modes,omitempty"`
	GrantedModes  []accessModeItem `json:"grantedModes,omitempty"`
	ExtendedModes []accessModeItem `json:"extendedModes,omitempty"`
	RoleUpgraded  bool             `json:"roleUpgraded,omitempty"`
	Role          string           `json:"role,omitempty"`
	RequiresAuth  bool             `json:"requiresAuth,omitempty"`
	Error         string           `json:"error,omitempty"`
	ErrorCode     string           `json:"errorCode,omitempty"`
}

type promoRow struct {
	ID                int64
	Code              string
	MaxUses           int64
	UsedCount         int64
	ActiveFrom        *time.Time
	ActiveTo          *time.Time
	DurationDays      int
	DurationRaw       string
	DailyMessageLimit *int64
	AccessPriority    int64
	GrantsType        string
	TargetID          int64
	TargetIDs         []int64
	FirstModeID       *int64
}

func (h Handler) AccessStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	userID, _, _, err := h.ensureGuestUser(r.Context(), w, r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}

	modes, err := h.getActiveAccessModes(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, accessStatusResponse{
		OK:          true,
		UserID:      userID,
		HasAccess:   len(modes) > 0,
		ActiveModes: modes,
		ActiveTo:    maxActiveTo(modes),
	})
}

func (h Handler) ApplyPromocode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, applyPromocodeResponse{OK: false, Error: "Метод не поддерживается", Code: "server_error"})
		return
	}

	// K-NEW3-1: rate limit against promocode brute force.
	// 30 attempts / 10 minutes per IP: enough for a legit user, not enough for guessing.
	if !h.allowAuthAttempt(r, "promocode", 30, 10*time.Minute) {
		writeJSON(w, http.StatusTooManyRequests, applyPromocodeResponse{OK: false, Error: "Слишком много попыток, подождите", Code: "rate_limited"})
		return
	}

	var currentUserID int64
	if user, err := h.currentUser(r.Context(), r); err == nil {
		currentUserID = user.ID
	} else if requestHasSessionCookie(r) {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusUnauthorized, applyPromocodeResponse{OK: false, Error: "auth required", ErrorCode: "auth_required", RequiresAuth: true})
		return
	}

	var req applyPromocodeRequest
	// W-2: cap the request body at 16 KB.
	if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, applyPromocodeResponse{OK: false, Error: "Некорректный запрос", Code: "server_error"})
		return
	}

	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		writeJSON(w, http.StatusBadRequest, applyPromocodeResponse{OK: false, Error: "Введите промокод", Code: "empty"})
		return
	}

	ctx := r.Context()

	// The guest user is resolved BEFORE the transaction. ensureGuestUser goes
	// through the pool, and calling it while the transaction holds a
	// connection needs a second one: once concurrent applies fill the pool,
	// every request waits for the others until the acquire timeout
	// (TestPromoGuestEnterprise_ApplyWorksWithSingleConnectionPool).
	var guestUserID int64
	if currentUserID == 0 {
		var err error
		guestUserID, _, _, err = h.ensureGuestUser(ctx, w, r)
		if err != nil {
			if errors.Is(err, errAuthRequired) {
				clearSessionCookie(w, r)
				writeJSON(w, http.StatusUnauthorized, applyPromocodeResponse{OK: false, Error: "auth required", ErrorCode: "auth_required", RequiresAuth: true})
			} else {
				writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
			}
			return
		}
	}

	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
		return
	}
	defer tx.Rollback(ctx)

	promo, err := getPromocodeForUpdate(ctx, tx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, applyPromocodeResponse{OK: false, Error: "Такого промокода нет", Code: "not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
		return
	}

	now := time.Now()
	if promo.ActiveFrom != nil && now.Before(*promo.ActiveFrom) {
		writeJSON(w, http.StatusBadRequest, applyPromocodeResponse{OK: false, Error: "Промокод еще не активен", ErrorCode: "not_started"})
		return
	}
	if promo.ActiveTo != nil && now.After(*promo.ActiveTo) {
		writeJSON(w, http.StatusBadRequest, applyPromocodeResponse{OK: false, Error: "Промокод истек", Code: "expired"})
		return
	}
	if promo.MaxUses > 0 && promo.UsedCount >= promo.MaxUses {
		writeJSON(w, http.StatusBadRequest, applyPromocodeResponse{OK: false, Error: "Лимит активаций исчерпан", Code: "limit_reached"})
		return
	}

	grantType := strings.ToLower(strings.TrimSpace(promo.GrantsType))
	roleUpgraded := false
	grantedModeIDs := []int64{}
	extendedModeIDs := []int64{}
	durationDays := promo.DurationDays
	if durationDays <= 0 {
		durationDays = 30
	}

	// The admin role is granted only to a signed-in user, never to a guest,
	// even though the guest row already exists at this point.
	if grantType == "admin_role" && currentUserID == 0 {
		writeJSON(w, http.StatusUnauthorized, applyPromocodeResponse{OK: false, Error: "Сначала войдите через Яндекс, затем промокод применится к аккаунту", ErrorCode: "auth_required", RequiresAuth: true})
		return
	}
	userID := currentUserID
	if userID == 0 {
		userID = guestUserID
	}

	used, err := promocodeAlreadyUsedByUser(ctx, tx, userID, promo.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
		return
	}
	if used {
		writeJSON(w, http.StatusBadRequest, applyPromocodeResponse{OK: false, Error: "Вы уже использовали этот промокод", ErrorCode: "already_used"})
		return
	}

	if grantType == "admin_role" {
		if err := grantAdminRole(ctx, tx, userID); err != nil {
			writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
			return
		}
		roleUpgraded = true
	} else {
		modeIDs, err := resolvePromocodeModeIDs(ctx, tx, promo)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
			return
		}
		if len(modeIDs) == 0 {
			writeJSON(w, http.StatusBadRequest, applyPromocodeResponse{OK: false, Error: "Промокод не привязан к доступным режимам", Code: "server_error"})
			return
		}
		grantedModeIDs, extendedModeIDs, err = grantPromocodeModeAccesses(ctx, tx, userID, modeIDs, promo, durationDays)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
			return
		}
	}

	_, err = tx.Exec(ctx, `insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`, userID, promo.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
		return
	}

	_, err = tx.Exec(ctx, `update promocodes set used_count = coalesce(used_count, 0) + 1, updated_at = now() where id = $1`, promo.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
		return
	}

	modes, err := h.getActiveAccessModes(ctx, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, applyPromocodeResponse{OK: false, Error: err.Error(), ErrorCode: "server_error"})
		return
	}

	grantedModes, extendedModes := splitAccessModesByIDs(modes, grantedModeIDs, extendedModeIDs)
	promoModes := mergeAccessModeLists(grantedModes, extendedModes)
	switch strings.ToLower(strings.TrimSpace(grantType)) {
	case "tariff", "tariffs", "plan":
		// The leading mode comes from the tariff itself, not from the promocode.
		targetIDs := uniquePositiveIDs(promo.TargetIDs)
		if len(targetIDs) == 0 && promo.TargetID > 0 {
			targetIDs = []int64{promo.TargetID}
		}
		if len(targetIDs) > 0 {
			var tariffFirstModeID *int64
			_ = h.DB.QueryRow(ctx, `select first_mode_id from tariffs where id = $1`, targetIDs[0]).Scan(&tariffFirstModeID)
			if tariffFirstModeID != nil && *tariffFirstModeID > 0 {
				promoModes = reorderFirstMode(promoModes, *tariffFirstModeID)
			}
		}
	default:
		if promo.FirstModeID != nil && *promo.FirstModeID > 0 {
			promoModes = reorderFirstMode(promoModes, *promo.FirstModeID)
		}
	}

	role := ""
	if roleUpgraded {
		role = "admin"
	}
	writeJSON(w, http.StatusOK, applyPromocodeResponse{
		OK:            true,
		Code:          promo.Code,
		AccessDays:    durationDays,
		Modes:         promoModes,
		GrantedModes:  grantedModes,
		ExtendedModes: extendedModes,
		RoleUpgraded:  roleUpgraded,
		Role:          role,
	})
}
