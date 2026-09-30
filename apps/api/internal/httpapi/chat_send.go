package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	defaultChatMessageMaxChars = 30 * 1024
	minChatMessageMaxChars     = 1
	maxChatMessageMaxChars     = 1024 * 1024
)

func (h Handler) chatMessageMaxChars(ctx context.Context) int {
	return intSetting(ctx, h, "chat_message_max_chars", defaultChatMessageMaxChars, minChatMessageMaxChars, maxChatMessageMaxChars)
}

func (h Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	ctx := r.Context()
	userID, _, currentDialogID, err := h.ensureGuestUser(ctx, w, r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}
	// Every AI call below (orchestration, the answer itself) carries this
	// user's text, so the data-protection policy must know whose it is.
	ctx = withDataSubject(ctx, userID)
	// K-NEW-8: burst limit on the chat. Protection against amplification (every message =
	// an OpenAI API call). 30/3sec ~= 10 msg/sec: unreachable for a human, but
	// it does not interfere with quota race tests (which send <=10 in parallel).
	// The daily quota (50 msg/day) is a separate mechanism.
	if !h.allowChatBurst(userID, 30, 3*time.Second) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "слишком частые запросы, подождите пару секунд", "code": "chat_burst_limit"})
		return
	}

	var req SendMessageRequest
	// W-2: cap the body at 1 MB to protect against OOM on huge requests.
	if err := decodeJSONStrict(w, r, 1<<20, &req); err != nil || strings.TrimSpace(req.Text) == "" {
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

	dialogID := req.DialogID
	if dialogID == 0 && currentDialogID != nil {
		dialogID = *currentDialogID
	}
	if dialogID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "dialog is not selected"})
		return
	}

	mode, err := getModeByDialogID(ctx, h.DB, userID, dialogID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": "access required", "code": "access_required"})
		return
	}
	dialogModeID := mode.ID

	mode, basicMode, err := h.modeForKnowledgeSelection(ctx, h.DB, userID, mode, req.KnowledgeModeIDs)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !basicMode && mode.ID != dialogModeID {
		_, _ = h.DB.Exec(ctx, `update users_dialogs set mode_id=$2 where id=$1`, dialogID, mode.ID)
	}

	cmd, err := h.DB.Exec(ctx, `
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
		  )`,
		userID,
		mode.ID,
		dialogID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	trackUsersUpdate(cmd.RowsAffected())

	cleanText := sanitizeUserText(req.Text)
	if cleanText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty message"})
		return
	}
	maxChars := h.chatMessageMaxChars(r.Context())
	if utf8.RuneCountInString(cleanText) > maxChars {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":       false,
			"error":    "message too long",
			"code":     "message_too_long",
			"maxChars": maxChars,
		})
		return
	}
	attachmentContexts, err := h.loadOwnedAttachmentContexts(ctx, userID, req.AttachmentIDs)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error(), "code": "invalid_attachment"})
		return
	}
	quotaBeforeSend, err := h.dailyQuota(ctx, userID, mode.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Atomic check-and-increment must happen BEFORE inserting the user message,
	// so a quota-exhausted send doesn't leave an orphan row in dialogs_messages.
	// tryIncrementDailyMessageCount handles concurrent callers via row-level
	// locking — see its doc comment.
	accessClaim, accessAllowed, err := h.tryClaimModeAccessUsage(ctx, userID, mode.ID, 1)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !accessAllowed {
		freshQuota, _ := h.dailyQuota(ctx, userID, mode.ID)
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "daily message limit exhausted", "code": "daily_quota_exhausted", "quota": freshQuota})
		return
	}
	claimedModeID := mode.ID
	claimedModeName := mode.Name
	var claimedAccessID *int64
	if accessClaim != nil {
		id := accessClaim.AccessID
		claimedAccessID = &id
	}
	limit := int64(0)
	if quotaBeforeSend != nil && quotaBeforeSend.Limit != nil {
		limit = *quotaBeforeSend.Limit
	}
	newUsed, allowed, err := h.tryIncrementDailyMessageCount(ctx, userID, limit, 1)
	if err != nil {
		if accessClaim != nil {
			h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !allowed {
		if accessClaim != nil {
			h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
		}
		// Race-safe: another concurrent request may have used the last slot
		// between dailyQuota read and our increment. Re-read for accurate response.
		freshQuota, _ := h.dailyQuota(ctx, userID, mode.ID)
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "daily message limit exhausted", "code": "daily_quota_exhausted", "quota": freshQuota})
		return
	}
	userMsg, err := insertDialogMessage(ctx, h.DB, dialogID, "user", cleanText)
	if err != nil {
		if accessClaim != nil {
			h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
		}
		h.decrementDailyMessageCount(ctx, userID, 1)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.recordDialogMessageAccessUsage(ctx, userID, claimedModeID, userMsg.ID, claimedAccessID, "message")
	userMsg.ModeID = &claimedModeID
	userMsg.ModeName = claimedModeName
	if err := linkMessageAttachments(ctx, h.DB, userMsg.ID, req.AttachmentIDs); err != nil {
		h.removeDialogMessageAccessUsage(ctx, userMsg.ID, claimedAccessID)
		if accessClaim != nil {
			h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
		}
		h.decrementDailyMessageCount(ctx, userID, 1)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	go func(modeName string) {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.forwardUserMessageToTelegram(notifyCtx, userID, dialogID, modeName, cleanText)
	}(mode.Name)

	responseMode := normalizeResponseMode(req.ResponseMode)
	if responseMode == "live" && !h.userCanUseLiveAI(ctx, userID) {
		responseMode = "test"
	}
	modeSwitched := false
	var switchTrace *ChatMessage
	responseAccessID := claimedAccessID
	if responseMode == "live" && !basicMode {
		if nextMode, switched, err := h.orchestrateMode(ctx, h.DB, userID, dialogID, mode, req.KnowledgeModeIDs); err == nil && switched {
			previousMode := mode
			mode = nextMode
			modeSwitched = true
			if id, accessErr := h.currentModeAccessID(ctx, userID, mode.ID); accessErr == nil && id != nil {
				responseAccessID = id
			}
			_, _ = h.DB.Exec(ctx, `update users_dialogs set mode_id=$2 where id=$1`, dialogID, mode.ID)
			if cmd, err := h.DB.Exec(ctx, `
				update users
				set current_mode = $2,
				    current_dialog = $3,
				    updated_at = now()
				where id = $1
				  and (
					current_mode is distinct from $2
					or current_dialog is distinct from $3
				  )`, userID, mode.ID, dialogID); err == nil {
				trackUsersUpdate(cmd.RowsAffected())
			}
			if msg, traceErr := insertModeSwitchTrace(ctx, h.DB, dialogID, previousMode.Name, mode.Name, true); traceErr == nil {
				switchTrace = msg
			}
		}
	}
	usedLive := false
	inputTokens := 0
	outputTokens := 0
	estimatedCost := 0.0
	assistantText := fmt.Sprintf("[TEST MODE]\nВы написали: %q\n\nСистема работает.\nТекущий режим: %s\nDialog ID: %d", cleanText, mode.Name, dialogID)
	if basicMode {
		assistantText = fmt.Sprintf("[TEST MODE]\nВы написали: %q\n\nРежим: Базовый ИИ. База знаний отключена, отвечаю без промпта режима.\nDialog ID: %d", cleanText, dialogID)
	}

	if responseMode == "live" {
		aiText := cleanText
		if attachmentContext := buildAttachmentContextForAI(attachmentContexts, h.chatAttachmentSettings(ctx).ContextMaxChars); attachmentContext != "" {
			aiText = cleanText + "\n\n" + attachmentContext
		}
		liveText, inTok, outTok, cost, err := h.callLiveAI(ctx, userID, dialogID, mode, aiText)
		if err != nil {
			// K-2: roll back the quota; the user must not lose a slot because of an AI failure.
			// The user's message is already inserted (the history is kept), the quota is not.
			h.decrementDailyMessageCount(ctx, userID, 1)
			h.removeDialogMessageAccessUsage(ctx, userMsg.ID, claimedAccessID)
			if accessClaim != nil {
				h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
			}
			if errors.Is(err, errCrossBorderConsentRequired) {
				writeJSON(w, http.StatusForbidden, map[string]any{
					"ok":    false,
					"error": "Чтобы ИИ ответил, нужно ваше согласие на передачу переписки за рубеж. Его можно дать в профиле.",
					"code":  "cross_border_consent_required",
				})
				return
			}
			log.Printf("chat send: live AI failed user=%d dialog=%d: %v", userID, dialogID, err)
			response := map[string]any{"ok": false, "error": liveAIUserMessage, "code": "live_ai_error"}
			if debug, ok := h.chatLiveAIDebug(ctx, userID, err); ok {
				response["error"] = err.Error()
				response["debug"] = debug
			}
			writeJSON(w, http.StatusBadGateway, response)
			return
		}
		usedLive = true
		assistantText = liveText
		inputTokens = inTok
		outputTokens = outTok
		estimatedCost = cost
	}

	assistantMsg, err := insertDialogMessage(ctx, h.DB, dialogID, "assistant", assistantText)
	if err != nil {
		h.removeDialogMessageAccessUsage(ctx, userMsg.ID, claimedAccessID)
		if accessClaim != nil {
			h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
		}
		h.decrementDailyMessageCount(ctx, userID, 1)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.recordDialogMessageAccessUsage(ctx, userID, mode.ID, assistantMsg.ID, responseAccessID, "message")
	responseModeID := mode.ID
	assistantMsg.ModeID = &responseModeID
	assistantMsg.ModeName = mode.Name

	// Lead trigger: the user's Nth reply in a mode with notifications
	// enabled -> a summary to the manager in Max (see lead_notifications.go).
	go func(modeID int64, modeName string) {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		// The lead summary is built from this user's dialog.
		notifyCtx = withDataSubject(notifyCtx, userID)
		h.maybeSendLeadNotification(notifyCtx, userID, dialogID, modeID, modeName)
	}(mode.ID, mode.Name)

	if usedLive {
		_, _ = h.DB.Exec(ctx, `
			insert into message_usage
				(user_id, mode_id, dialog_message_id, input_tokens, output_tokens, total_tokens, estimated_cost, access_id, is_compressed_response)
			values
				($1,$2,$3,$4,$5,$6,$7,$8,false)`,
			userID,
			mode.ID,
			assistantMsg.ID,
			inputTokens,
			outputTokens,
			inputTokens+outputTokens,
			nullableCost(estimatedCost),
			responseAccessID,
		)
	}

	updatedQuota := quotaBeforeSend
	if quotaBeforeSend != nil {
		limit := quotaBeforeSend.Limit
		remaining := int64(0)
		if limit != nil {
			remaining = *limit - newUsed
			if remaining < 0 {
				remaining = 0
			}
		}
		updatedQuota = &DailyQuota{Limit: limit, Used: newUsed, Remaining: &remaining}
	}
	writeJSON(w, http.StatusOK, SendMessageResponse{OK: true, User: userMsg, SwitchTrace: switchTrace, Assistant: assistantMsg, UsedLive: usedLive, ModeID: mode.ID, ModeName: mode.Name, ModeSwitched: modeSwitched, Quota: updatedQuota, Attachments: uniquePositiveInt64s(req.AttachmentIDs)})
}
