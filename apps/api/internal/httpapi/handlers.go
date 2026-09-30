package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB *pgxpool.Pool
	// OpenAIAPIKey is the openai-protocol key for the CURRENT call. It is not filled
	// from env: withGateway sets it from the ai_gateways registry row
	// (and tests set it directly). Empty means the vsegpt gateway is not configured.
	OpenAIAPIKey        string
	OpenAIBaseURL       string
	PromoAdminSecret    string
	HTTPClient          *http.Client
	TelegramBotToken    string
	TelegramChatID      string
	TelegramAPIBaseURL  string // Bot API base; in prod a proxy (api.telegram.org is blocked in Russia)
	TelegramBotUsername string // bot username for the t.me/<bot>?start= deeplink
	YooKassaShopID      string
	YooKassaSecretKey   string
	YooKassaWebhookPath string // if empty, the webhook is disabled
	GuestCookieSecret   string // L-1: HMAC-SHA256 key for the guest cookie; empty = unsigned (dev/tests)
	// TrustedProxyIPs (S-NEW-4): IP/CIDR of trusted reverse proxies. Only these RemoteAddr values
	// may set X-Forwarded-For / X-Real-IP. Empty = trust everyone (dev/tests).
	TrustedProxyIPs          []string
	MetricsBasicAuth         string
	BootstrapAdminToken      string
	AuthDevReturnResetToken  bool
	AuthDevReturnVerifyToken bool
	YandexClientID           string
	YandexClientSecret       string
	APIPublicBaseURL         string
	MaxBotToken              string
	MaxBotUsername           string
	// MaxWebhookSecret: see config.Config.MaxWebhookSecret and max_messenger.go.
	MaxWebhookSecret string
	// ModeHistory is the git mirror of edits to modes' AI prompts (nil or
	// an empty key = the feature is silently off). See mode_history.go.
	ModeHistory modeHistoryStore
	// Direct AI providers (multi-provider routing, see ai_providers.go).
	// An empty key = the provider is not configured and drops out of the fallback chain.
	GeminiAPIKey     string
	GeminiAPIBaseURL string
	// Vertex AI is an alternative path to Gemini, enabled ONLY by setting
	// GeminiVertexSAJSON (a service account key, raw JSON or base64).
	// Why it is needed and why a geo-ban cannot be fixed by routing: see the header of
	// ai_gemini_vertex.go. Empty = the old way through AI Studio.
	GeminiVertexSAJSON   string
	GeminiVertexProject  string
	GeminiVertexLocation string
	// GeminiVertexBaseURL overrides the base entirely; needed in prod because
	// *-aiplatform.googleapis.com is unreachable from Russia and we must go through the relay.
	GeminiVertexBaseURL string
	AnthropicAPIKey     string
	AnthropicAPIBaseURL string
	// gatewayKeyOverride is the key of a specific registry gateway for ONE call.
	// It is set only in the Handler copy from withGateway (ai_gateways.go) and
	// overrides all other key sources; in the main handler it is always
	// empty. The field is unexported on purpose: there is nothing outside that could set it.
	gatewayKeyOverride string
	// DisableRateLimits turns off the rate ceilings. Set only by tests.
	DisableRateLimits bool
	// DataProtectionProfile is the legal regime of the installation
	// (none|ru|eu|us), see data_protection.go.
	DataProtectionProfile string
	// DataStorageCountry is the operator's declaration of where the main
	// database is hosted (ISO 3166-1 alpha-2). Used only for warnings.
	DataStorageCountry string
	// c holds all per-Handler caches, isolated between tests.
	c *handlerCaches
}

// modeHistoryOrNoop returns ModeHistory if it is configured, or a no-op
// stub (empty history, no commits), so calling code does not
// have to check for nil at every step.
func (h Handler) modeHistoryOrNoop() modeHistoryStore {
	if h.ModeHistory != nil {
		return h.ModeHistory
	}
	return noopModeHistoryStore{}
}

// noopModeHistoryStore is used when ModeHistory is not configured
// (nil): all operations silently do nothing, as the spec requires ("keys
// not set -> the feature is a no-op").
type noopModeHistoryStore struct{}

func (noopModeHistoryStore) configured() bool { return false }
func (noopModeHistoryStore) commitSnapshot(context.Context, int64, modeSnapshot, string, string, string) (string, error) {
	return "", nil
}
func (noopModeHistoryStore) history(context.Context, int64, int) ([]modeHistoryEntry, error) {
	return nil, nil
}
func (noopModeHistoryStore) snapshotAt(context.Context, int64, string) (modeSnapshot, error) {
	return modeSnapshot{}, errModeHistoryNotConfigured
}

const guestCookieName = "mindstrata_guest_user_id"

const modesCacheTTL = 5 * time.Minute

func cloneModeOptions(src []ModeOption) []ModeOption {
	dst := make([]ModeOption, len(src))
	copy(dst, src)
	return dst
}

func (h Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "api"})
}

func (h Handler) DBCheck(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := h.DB.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h Handler) StartChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	ctx := r.Context()

	// Auth path: single query returns id+role+current_mode+current_dialog.
	// Guest path: falls back to ensureGuestUser (cookie lookup or new guest creation).
	role := ""
	var userID int64
	var currentModeID, currentDialogID *int64

	if authUser, err := h.currentUserFull(ctx, r); err == nil && authUser.ID > 0 {
		userID = authUser.ID
		role = authUser.Role
		currentModeID = authUser.CurrentModeID
		currentDialogID = authUser.CurrentDialogID
	} else if requestHasSessionCookie(r) {
		clearSessionCookie(w, r)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "auth required"})
		return
	} else {
		var guestErr error
		userID, currentModeID, currentDialogID, guestErr = h.ensureGuestUser(ctx, w, r)
		if guestErr != nil {
			writeGuestAuthError(w, r, guestErr)
			return
		}
		role, _ = h.userRole(ctx, userID)
	}

	modes, err := h.listUserModes(ctx, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if len(modes) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "access required", "code": "access_required"})
		return
	}
	modes = h.addQuotasToModes(ctx, userID, modes)
	currentModeID, currentDialogID = h.ensureDefaultChatSelection(ctx, userID, currentModeID, currentDialogID, modes)

	// Extract quota for the selected mode from addQuotasToModes — already computed, no extra query.
	quota := (*DailyQuota)(nil)
	if currentModeID != nil {
		for _, m := range modes {
			if m.ID == *currentModeID {
				quota = m.Quota
				break
			}
		}
	}
	// Inline the first page of the active dialog so the web app does not need
	// a second round-trip to /api/chat/history before it can paint messages.
	messages := []ChatMessage(nil)
	if currentDialogID != nil {
		if recent, err := getDialogMessages(ctx, h.DB, *currentDialogID, 20, 0); err == nil {
			messages = recent
		}
	}

	var maxChatID *int64
	if role != "guest" && role != "" {
		_ = h.DB.QueryRow(ctx, `SELECT max_chat_id FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&maxChatID)
	}
	writeJSON(w, http.StatusOK, StartResponse{
		OK:                  true,
		UserID:              userID,
		Role:                role,
		CurrentModeID:       currentModeID,
		CurrentDialog:       currentDialogID,
		ChatMessageMaxChars: h.chatMessageMaxChars(ctx),
		Modes:               modes,
		Quota:               quota,
		Messages:            messages,
		MaxLinked:           maxChatID != nil,
		MaxBotLink:          h.maxBotStartLink(userID),
	})
}

func (h Handler) ensureDefaultChatSelection(ctx context.Context, userID int64, currentModeID, currentDialogID *int64, modes []ModeOption) (*int64, *int64) {
	if currentDialogID != nil {
		if mode, err := getModeByDialogID(ctx, h.DB, userID, *currentDialogID); err == nil {
			modeID := mode.ID
			return &modeID, currentDialogID
		}
	}

	if len(modes) == 0 {
		return nil, nil
	}

	mode, err := getUserAccessibleModeByID(ctx, h.DB, userID, modes[0].ID)
	if err != nil {
		return nil, nil
	}

	var dialogID int64
	if err := h.DB.QueryRow(ctx, `insert into users_dialogs (user_id, mode_id) values ($1,$2) returning id`, userID, mode.ID).Scan(&dialogID); err != nil {
		return nil, nil
	}
	if cmd, err := h.DB.Exec(ctx, `
		update users
		set current_mode = $2,
		    current_dialog = $3,
		    accepted_tos = true,
		    updated_at = now()
		where id = $1
		  and (
			current_mode is distinct from $2
			or current_dialog is distinct from $3
			or accepted_tos is distinct from true
		  )`, userID, mode.ID, dialogID); err == nil {
		trackUsersUpdate(cmd.RowsAffected())
	}
	return &mode.ID, &dialogID
}

func (h Handler) SelectMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	ctx := r.Context()
	userID, _, _, err := h.ensureGuestUser(ctx, w, r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}
	var req SelectModeRequest
	// W-2: cap the body at 64 KB.
	if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if ok, err := h.userHasActiveMode(ctx, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	} else if !ok {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "access required", "code": "access_required"})
		return
	}

	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)

	mode, err := h.baseDialogMode(ctx, tx, userID, req.ModeID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "mode not found"})
		return
	}
	responseModeName := mode.Name
	responseWelcome := mode.WelcomeMessage
	if req.ModeID == 0 {
		responseModeName = "Базовый ИИ"
		responseWelcome = ""
	}

	dialogID := int64(0)
	var previousModeID int64
	previousModeName := ""
	if !req.NewDialog {
		_ = tx.QueryRow(ctx, `
			select d.id, d.mode_id, coalesce(m.name, 'Базовый ИИ')
			from users_dialogs d
			left join modes m on m.id = d.mode_id
			where d.user_id = $1 and d.deleted_at is null
			order by case when d.id = (select current_dialog from users where id = $1) then 0 else 1 end, d.id desc
			limit 1`, userID).Scan(&dialogID, &previousModeID, &previousModeName)
	}

	var switchTrace *ChatMessage
	if dialogID > 0 {
		_, err = tx.Exec(ctx, `update users_dialogs set mode_id=$2 where id=$1 and user_id=$3`, dialogID, mode.ID, userID)
		if err == nil && previousModeID > 0 && previousModeID != mode.ID {
			switchTrace, _ = insertModeSwitchTrace(ctx, tx, dialogID, previousModeName, responseModeName, false)
		}
	} else {
		err = tx.QueryRow(ctx, `insert into users_dialogs (user_id, mode_id) values ($1,$2) returning id`, userID, mode.ID).Scan(&dialogID)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	cmd, err := tx.Exec(ctx, `
		update users
		set current_mode = $2,
		    current_dialog = $3,
		    accepted_tos = true,
		    updated_at = now()
		where id = $1
		  and (
			current_mode is distinct from $2
			or current_dialog is distinct from $3
			or accepted_tos is distinct from true
		  )`, userID, mode.ID, dialogID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	trackUsersUpdate(cmd.RowsAffected())

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	quota, _ := h.dailyQuota(ctx, userID, mode.ID)
	writeJSON(w, http.StatusOK, SelectModeResponse{
		OK: true, UserID: userID, DialogID: dialogID, ModeID: mode.ID, ModeName: responseModeName, WelcomeMessage: responseWelcome, Quota: quota, SwitchTrace: switchTrace,
	})
}
