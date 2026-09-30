package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

func (h Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		h.DeleteHistory(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	ctx := r.Context()
	userID, _, currentDialogID, err := h.ensureGuestUser(ctx, w, r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}
	if currentDialogID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "messages": []ChatMessage{}})
		return
	}
	dialogID := *currentDialogID
	if q := r.URL.Query().Get("dialogId"); q != "" {
		if parsed, err := strconv.ParseInt(q, 10, 64); err == nil {
			dialogID = parsed
		}
	}
	limit := int64(10)
	if q := r.URL.Query().Get("limit"); q != "" {
		if parsed, err := strconv.ParseInt(q, 10, 64); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	beforeID := int64(0)
	if q := r.URL.Query().Get("beforeId"); q != "" {
		if parsed, err := strconv.ParseInt(q, 10, 64); err == nil {
			beforeID = parsed
		}
	}
	mode, hasActiveAccess, err := getDialogModeByID(ctx, h.DB, userID, dialogID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": "dialog not found"})
		return
	}
	msgs, err := getDialogMessages(ctx, h.DB, dialogID, limit, beforeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"dialogId":       dialogID,
		"modeId":         mode.ID,
		"modeName":       mode.Name,
		"messages":       msgs,
		"accessActive":   hasActiveAccess,
		"accessRequired": !hasActiveAccess,
	})
}

func (h Handler) DeleteHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, _, _, err := h.ensureGuestUser(ctx, w, r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}

	if strings.EqualFold(r.URL.Query().Get("all"), "true") {
		cmd, err := h.DB.Exec(ctx, `
			update users_dialogs
			set deleted_at = coalesce(deleted_at, now())
			where user_id = $1 and deleted_at is null`, userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, _ = h.DB.Exec(ctx, `update users set current_dialog = null, updated_at = now() where id = $1`, userID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": cmd.RowsAffected()})
		return
	}

	dialogID, err := strconv.ParseInt(r.URL.Query().Get("dialogId"), 10, 64)
	if err != nil || dialogID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "dialogId is required"})
		return
	}
	cmd, err := h.DB.Exec(ctx, `
		update users_dialogs
		set deleted_at = coalesce(deleted_at, now())
		where id = $1 and user_id = $2 and deleted_at is null`, dialogID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = h.DB.Exec(ctx, `update users set current_dialog = null, updated_at = now() where id = $1 and current_dialog = $2`, userID, dialogID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": cmd.RowsAffected()})
}

func (h Handler) CompleteDialog(w http.ResponseWriter, r *http.Request) {
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

	var req CompleteRequest
	// W-2: cap the body at 16 KB; the error is ignored since dialogID is optional.
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req)

	dialogID := req.DialogID
	if dialogID == 0 && currentDialogID != nil {
		dialogID = *currentDialogID
	}
	if dialogID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no active dialog"})
		return
	}

	mode, err := getModeByDialogID(ctx, h.DB, userID, dialogID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": "dialog not found"})
		return
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

	quotaBeforeSummary, err := h.dailyQuota(ctx, userID, mode.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
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
	var accessID *int64
	if accessClaim != nil {
		id := accessClaim.AccessID
		accessID = &id
	}
	// Atomic claim of the quota slot — same race-safety as SendMessage.
	// We claim BEFORE the live AI call so a parallel sender can't slip in
	// and exhaust the counter while our summary is being computed.
	summaryLimit := int64(0)
	if quotaBeforeSummary != nil && quotaBeforeSummary.Limit != nil {
		summaryLimit = *quotaBeforeSummary.Limit
	}
	newUsed, allowed, err := h.tryIncrementDailyMessageCount(ctx, userID, summaryLimit, 1)
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
		freshQuota, _ := h.dailyQuota(ctx, userID, mode.ID)
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "daily message limit exhausted", "code": "daily_quota_exhausted", "quota": freshQuota})
		return
	}

	messages, err := getDialogMessages(ctx, h.DB, dialogID, 50, 0)

	if err != nil {
		if accessClaim != nil {
			h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
		}
		h.decrementDailyMessageCount(ctx, userID, 1)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	summaryText := buildTestSummary(mode.Name, dialogID, messages)
	usedLive := false
	inputTokens := 0
	outputTokens := 0
	estimatedCost := 0.0
	responseMode := normalizeResponseMode(req.ResponseMode)
	if responseMode == "live" && !h.userCanUseLiveAI(ctx, userID) {
		responseMode = "test"
	}
	if responseMode == "live" {
		liveText, inTok, outTok, cost, err := h.callLiveAISummary(ctx, userID, mode, messages)
		if err != nil {
			// K-2 (CompleteDialog): roll back the quota; the user must not
			// lose a slot because of an AI failure. Symmetric to SendMessage.
			h.decrementDailyMessageCount(ctx, userID, 1)
			if accessClaim != nil {
				h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
			}
			response := map[string]any{"ok": false, "error": err.Error(), "code": "live_ai_error"}
			if debug, ok := h.chatLiveAIDebug(ctx, userID, err); ok {
				response["debug"] = debug
			}
			writeJSON(w, http.StatusBadGateway, response)
			return
		}
		summaryText = liveText
		usedLive = true
		inputTokens = inTok
		outputTokens = outTok
		estimatedCost = cost
	}
	msg, err := insertDialogMessage(ctx, h.DB, dialogID, "summary", summaryText)
	if err != nil {
		if accessClaim != nil {
			h.decrementModeAccessUsage(ctx, userID, claimedModeID, accessClaim.AccessID, 1)
		}
		h.decrementDailyMessageCount(ctx, userID, 1)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.recordDialogMessageAccessUsage(ctx, userID, claimedModeID, msg.ID, accessID, "summary")
	// newUsed already obtained from tryIncrementDailyMessageCount above.

	if usedLive {
		_, _ = h.DB.Exec(ctx, `
			insert into message_usage
				(user_id, mode_id, dialog_message_id, input_tokens, output_tokens, total_tokens, estimated_cost, access_id, is_compressed_response)
			values
				($1,$2,$3,$4,$5,$6,$7,$8,false)`,
			userID,
			mode.ID,
			msg.ID,
			inputTokens,
			outputTokens,
			inputTokens+outputTokens,
			nullableCost(estimatedCost),
			accessID,
		)
	}

	updatedQuota := quotaBeforeSummary
	if quotaBeforeSummary != nil {
		limit := quotaBeforeSummary.Limit
		remaining := int64(0)
		if limit != nil {
			remaining = *limit - newUsed
			if remaining < 0 {
				remaining = 0
			}
		}
		updatedQuota = &DailyQuota{Limit: limit, Used: newUsed, Remaining: &remaining}
	}
	writeJSON(w, http.StatusOK, CompleteResponse{OK: true, Summary: msg, UsedLive: usedLive, Quota: updatedQuota})
}

// filterBiDiAndControl removes BiDi control characters and zero-width chars.
// S-5: prevents Unicode spoofing in user text.
func filterBiDiAndControl(r rune) rune {
	switch r {
	case '\u200e', '\u200f',
		'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
		'\u2066', '\u2067', '\u2068', '\u2069',
		'\u200b', '\u200c', '\u200d',
		'\ufeff':
		return -1
	}
	if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
		return -1
	}
	return r
}

func sanitizeUserText(raw string) string {
	// S-5: NFC normalisation + BiDi/zero-width filter before regular processing
	raw = norm.NFC.String(raw)
	raw = strings.Map(filterBiDiAndControl, raw)

	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func (h Handler) forwardUserMessageToTelegram(ctx context.Context, userID, dialogID int64, modeName, text string) error {
	if strings.TrimSpace(h.TelegramBotToken) == "" || strings.TrimSpace(h.TelegramChatID) == "" {
		return nil
	}

	payload := map[string]any{
		"chat_id": h.TelegramChatID,
		"text":    fmt.Sprintf("📩 Новое сообщение\nUser: %d\nDialog: %d\nРежим: %s\n\n%s", userID, dialogID, modeName, text),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", h.telegramAPIBase(), h.TelegramBotToken), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram request: %w", redactSecretInError(err, h.TelegramBotToken))
	}
	req.Header.Set("Content-Type", "application/json")

	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send: %w", redactSecretInError(err, h.TelegramBotToken))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("telegram send failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return nil
}
