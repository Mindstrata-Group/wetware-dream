package httpapi

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type modeRow struct {
	ID                        int64
	Name                      string
	Prompt                    string
	WelcomeMessage            string
	AIModel                   string
	AIProvider                string // vsegpt | gemini | anthropic
	ThinkingMode              string // default | off | low | high
	Temperature               float64
	Criteria                  string
	OrchestratorCheckInterval int
	MaxTokens                 int64 // modes.ai_max_tokens, 0 = the global default
}

func (h Handler) ensureGuestUser(ctx context.Context, w http.ResponseWriter, r *http.Request) (int64, *int64, *int64, error) {
	if h.DB == nil {
		return 0, nil, nil, errors.New("database is not configured")
	}
	if authUser, err := h.currentUserFull(ctx, r); err == nil && authUser.ID > 0 {
		return authUser.ID, authUser.CurrentModeID, authUser.CurrentDialogID, nil
	} else if requestHasSessionCookie(r) {
		return 0, nil, nil, errAuthRequired
	}
	// L-1: use the HMAC-verified function instead of an inline ParseInt
	if guestID := guestUserIDFromRequest(r, h.GuestCookieSecret); guestID > 0 {
		var userID int64
		var currentModeID, currentDialogID *int64
		row := h.DB.QueryRow(ctx, `select id, current_mode, current_dialog from users where id=$1`, guestID)
		if err := row.Scan(&userID, &currentModeID, &currentDialogID); err == nil {
			return userID, currentModeID, currentDialogID, nil
		}
	}
	if !h.allowAuthAttempt(r, "guest", 20, 24*time.Hour) {
		return 0, nil, nil, errors.New("too many guest accounts from this network today")
	}
	// W-5: crypto/rand instead of math/rand so the guest name is unpredictable
	suffixBuf := make([]byte, 4)
	_, _ = cryptorand.Read(suffixBuf)
	username := fmt.Sprintf("webguest_%d_%s", time.Now().Unix(), hex.EncodeToString(suffixBuf))
	var userID int64
	err := h.DB.QueryRow(ctx, `insert into users (telegram_id, telegram_username, accepted_tos) values (null, $1, true) returning id`, username).Scan(&userID)
	if err != nil {
		return 0, nil, nil, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     guestCookieName,
		Value:    h.guestCookieValue(userID), // L-1: signed value
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPSRequest(r), // S-NEW-6: consistency with the session cookie
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	})
	return userID, nil, nil, nil
}

func (h Handler) userHasActiveMode(ctx context.Context, userID int64) (bool, error) {
	var ok bool
	err := h.DB.QueryRow(ctx, `
		select exists(
			select 1
			from user_mode_access uma
			join modes m on m.id = uma.mode_id
			where uma.user_id = $1
			  and (uma.active_from is null or uma.active_from <= now())
			  and (uma.active_to is null or uma.active_to >= now())
			  and m.hidden_at is null
		)`, userID).Scan(&ok)
	return ok, err
}

func (h Handler) clearModesCache() {
	h.c.modes.Lock()
	h.c.modes.modes = nil
	h.c.modes.expiresAt = time.Time{}
	h.c.modes.Unlock()
}

func (h Handler) listModes(ctx context.Context) ([]ModeOption, error) {
	now := time.Now()

	h.c.modes.RLock()
	if h.c.modes.modes != nil && now.Before(h.c.modes.expiresAt) {
		recordCacheLookup("modes", true)
		out := cloneModeOptions(h.c.modes.modes)
		h.c.modes.RUnlock()
		return out, nil
	}
	h.c.modes.RUnlock()
	recordCacheLookup("modes", false)

	rows, err := h.DB.Query(ctx, `
		select id, name, coalesce(welcome_message, '')
		from modes
		where hidden_at is null
		  and (
		    not exists (select 1 from tariff_mode tm where tm.mode_id = modes.id)
		    or exists (select 1 from tariff_mode tm join tariffs t on t.id = tm.tariff_id where tm.mode_id = modes.id and t.archived_at is null)
		  )
		order by id asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ModeOption, 0, 120)
	for rows.Next() {
		var item ModeOption
		if err := rows.Scan(&item.ID, &item.Name, &item.WelcomeMessage); err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	h.c.modes.Lock()
	h.c.modes.modes = cloneModeOptions(out)
	h.c.modes.expiresAt = time.Now().Add(modesCacheTTL)
	h.c.modes.Unlock()

	return out, nil
}

func (h Handler) listUserModes(ctx context.Context, userID int64) ([]ModeOption, error) {
	rows, err := h.DB.Query(ctx, `
		select distinct m.id, m.name, coalesce(m.welcome_message, '')
		from modes m
		join user_mode_access uma on uma.mode_id = m.id
		where uma.user_id = $1
		  and (uma.active_from is null or uma.active_from <= now())
		  and (uma.active_to is null or uma.active_to >= now())
		  and m.hidden_at is null
		order by m.id asc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ModeOption, 0, 32)
	for rows.Next() {
		var item ModeOption
		if err := rows.Scan(&item.ID, &item.Name, &item.WelcomeMessage); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h Handler) baseDialogMode(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID, requestedModeID int64) (modeRow, error) {
	if requestedModeID > 0 {
		return getUserAccessibleModeByID(ctx, q, userID, requestedModeID)
	}

	var currentModeID int64
	err := q.QueryRow(ctx, `
		select coalesce(u.current_mode, 0)
		from users u
		where u.id = $1`, userID).Scan(&currentModeID)
	if err == nil && currentModeID > 0 {
		if mode, modeErr := getUserAccessibleModeByID(ctx, q, userID, currentModeID); modeErr == nil {
			return mode, nil
		}
	}

	return firstUserAccessibleMode(ctx, q, userID)
}

func firstUserAccessibleMode(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID int64) (modeRow, error) {
	var m modeRow
	err := q.QueryRow(ctx, `
		select m.id, m.name, m.prompt, coalesce(m.welcome_message,''), m.ai_model, coalesce(m.ai_provider, 'vsegpt'), coalesce(m.thinking_mode, 'default'), m.model_temperature, coalesce(m.criteria, ''), coalesce(m.orchestrator_check_interval, 5), coalesce(m.ai_max_tokens, 0)
		from modes m
		join user_mode_access uma on uma.mode_id = m.id
		where uma.user_id = $1
		  and (uma.active_from is null or uma.active_from <= now())
		  and (uma.active_to is null or uma.active_to >= now())
		  and m.hidden_at is null
		order by coalesce(uma.priority, 0) desc, m.id asc
		limit 1`, userID).Scan(&m.ID, &m.Name, &m.Prompt, &m.WelcomeMessage, &m.AIModel, &m.AIProvider, &m.ThinkingMode, &m.Temperature, &m.Criteria, &m.OrchestratorCheckInterval, &m.MaxTokens)
	return m, err
}

func (h Handler) modeForKnowledgeSelection(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID int64, current modeRow, knowledgeModeIDs []int64) (modeRow, bool, error) {
	ids := uniquePositiveIDs(knowledgeModeIDs)
	if len(ids) == 0 {
		current.Name = "Базовый ИИ"
		current.Prompt = ""
		current.WelcomeMessage = ""
		return current, true, nil
	}

	for _, id := range ids {
		if id == current.ID {
			return current, false, nil
		}
	}

	primaryID := ids[0]
	primaryMode, err := getUserAccessibleModeByID(ctx, q, userID, primaryID)
	if err == nil {
		return primaryMode, false, nil
	}

	return modeRow{}, false, fmt.Errorf("текущий режим недоступен в выбранной базе знаний")
}

func getModeByID(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, modeID int64) (modeRow, error) {
	var m modeRow
	err := q.QueryRow(ctx, `select id, name, prompt, coalesce(welcome_message,''), ai_model, coalesce(ai_provider, 'vsegpt'), coalesce(thinking_mode, 'default'), model_temperature, coalesce(criteria, ''), coalesce(orchestrator_check_interval, 5), coalesce(ai_max_tokens, 0) from modes where id=$1 and hidden_at is null`, modeID).Scan(&m.ID, &m.Name, &m.Prompt, &m.WelcomeMessage, &m.AIModel, &m.AIProvider, &m.ThinkingMode, &m.Temperature, &m.Criteria, &m.OrchestratorCheckInterval, &m.MaxTokens)
	return m, err
}

func getUserAccessibleModeByID(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID, modeID int64) (modeRow, error) {
	var m modeRow
	err := q.QueryRow(ctx, `
		select m.id, m.name, m.prompt, coalesce(m.welcome_message,''), m.ai_model, coalesce(m.ai_provider, 'vsegpt'), coalesce(m.thinking_mode, 'default'), m.model_temperature, coalesce(m.criteria, ''), coalesce(m.orchestrator_check_interval, 5), coalesce(m.ai_max_tokens, 0)
		from modes m
		where m.id = $1
		  and m.hidden_at is null
		  and exists (
			select 1
			from user_mode_access uma
			where uma.user_id = $2
			  and uma.mode_id = m.id
			  and (uma.active_from is null or uma.active_from <= now())
			  and (uma.active_to is null or uma.active_to >= now())
		  )`, modeID, userID).Scan(&m.ID, &m.Name, &m.Prompt, &m.WelcomeMessage, &m.AIModel, &m.AIProvider, &m.ThinkingMode, &m.Temperature, &m.Criteria, &m.OrchestratorCheckInterval, &m.MaxTokens)
	return m, err
}
