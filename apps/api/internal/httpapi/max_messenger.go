package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxUserToken is an HMAC token that identifies the user when the bot starts.
// Payload format: "{userID}_{hmac[:16]}"
func (h Handler) maxUserToken(userID int64) string {
	mac := hmac.New(sha256.New, []byte(h.MaxBotToken))
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(userID))
	mac.Write(b)
	sum := fmt.Sprintf("%x", mac.Sum(nil))
	return fmt.Sprintf("%d_%s", userID, sum[:16])
}

// maxParseToken parses the token and returns userID if the HMAC is valid.
func (h Handler) maxParseToken(token string) (int64, bool) {
	parts := strings.SplitN(token, "_", 2)
	if len(parts) != 2 {
		return 0, false
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || userID <= 0 {
		return 0, false
	}
	expected := h.maxUserToken(userID)
	expectedParts := strings.SplitN(expected, "_", 2)
	if len(expectedParts) != 2 {
		return 0, false
	}
	if !hmac.Equal([]byte(parts[1]), []byte(expectedParts[1])) {
		return 0, false
	}
	return userID, true
}

// maxBotStartLink returns the deeplink to start the bot bound to userID.
func (h Handler) maxBotStartLink(userID int64) string {
	if strings.TrimSpace(h.MaxBotUsername) == "" {
		return ""
	}
	// The official MAX deeplink format: https://max.ru/<botName>?start=<payload>
	// (no /bot/, the parameter is exactly start, payload up to 128 characters). See
	// https://dev.max.ru/help/deeplinks: when followed, the bot starts and sends
	// a bot_started event with this payload.
	return fmt.Sprintf("https://max.ru/%s?start=%s", h.MaxBotUsername, h.maxUserToken(userID))
}

// MaxNotificationsStartLink handles GET /api/notifications/max/start-link.
// Returns the link to start the bot with account binding.
func (h Handler) MaxNotificationsStartLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}

	var maxChatID *int64
	var maxBonusGranted bool
	_ = h.DB.QueryRow(r.Context(), `SELECT max_chat_id, max_bonus_granted FROM users WHERE id = $1 AND deleted_at IS NULL`, user.ID).
		Scan(&maxChatID, &maxBonusGranted)

	link := h.maxBotStartLink(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"startLink":       link,
		"botUsername":     h.MaxBotUsername,
		"maxLinked":       maxChatID != nil,
		"maxBonusGranted": maxBonusGranted,
	})
}

// maskEmailLabel masks an email for the "which other accounts this messenger
// is bound to" display: ab***@domain. Non-email labels (account #N) are returned as is.
func maskEmailLabel(label string) string {
	at := strings.IndexByte(label, '@')
	if at <= 0 {
		return label
	}
	visible := 2
	if at < visible {
		visible = at
	}
	return label[:visible] + "***" + label[at:]
}

// maxUnlinkCooldown is how long notifications cannot be unbound after binding.
const maxUnlinkCooldown = 24 * time.Hour

// MaxNotificationsUnlink handles POST /api/notifications/max/unlink.
// Unbinds Max from the account. Unbinding is forbidden for the first day after binding.
// max_bonus_granted is NOT reset: reconnecting will not grant the bonus again.
func (h Handler) MaxNotificationsUnlink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}

	var maxChatID *int64
	var linkedAt *time.Time
	if err := h.DB.QueryRow(r.Context(),
		`SELECT max_chat_id, max_linked_at FROM users WHERE id = $1 AND deleted_at IS NULL`,
		user.ID).Scan(&maxChatID, &linkedAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db error"})
		return
	}
	if maxChatID == nil {
		// Already unbound: idempotent success.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "maxLinked": false})
		return
	}
	if linkedAt != nil {
		if elapsed := time.Since(*linkedAt); elapsed < maxUnlinkCooldown {
			hoursLeft := int(math.Ceil((maxUnlinkCooldown - elapsed).Hours()))
			if hoursLeft < 1 {
				hoursLeft = 1
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":        false,
				"code":      "cooldown",
				"hoursLeft": hoursLeft,
				"error":     "Отвязать уведомления можно только через сутки после подключения.",
			})
			return
		}
	}
	if _, err := h.DB.Exec(r.Context(),
		`UPDATE users SET max_chat_id = NULL, max_linked_at = NULL, updated_at = now() WHERE id = $1`,
		user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "maxLinked": false})
}

// validMaxWebhookSecret: 32..256 chars of [A-Za-z0-9_-] (Max accepts 5..256 of
// these characters for a subscription secret; we require a strong one).
func validMaxWebhookSecret(s string) bool {
	if len(s) < 32 || len(s) > 256 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// maxUpdate is an incoming update from the Max Bot API.
type maxUpdate struct {
	UpdateType string      `json:"update_type"`
	Timestamp  int64       `json:"timestamp"`
	ChatID     int64       `json:"chat_id"`
	User       maxUserInfo `json:"user"`
	Payload    string      `json:"payload"` // startpayload from /start
	Message    *maxMessage `json:"message,omitempty"`
}

type maxUserInfo struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
}

type maxMessage struct {
	Body *maxMessageBody `json:"body,omitempty"`
}

type maxMessageBody struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Payload string `json:"payload"`
}

// MaxWebhook handles POST /webhooks/max/{secret}.
// Accepts updates from the Max Bot API. On bot_started with a valid
// payload it binds the user's account and grants +1 day of access.
func (h Handler) MaxWebhook(w http.ResponseWriter, r *http.Request, secret string) {
	// Both the path and the X-Max-Bot-Api-Secret header must carry the
	// configured secret; compared in constant time.
	if strings.TrimSpace(h.MaxBotToken) == "" || !validMaxWebhookSecret(h.MaxWebhookSecret) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false})
		return
	}
	want := []byte(h.MaxWebhookSecret)
	if subtle.ConstantTimeCompare([]byte(secret), want) != 1 ||
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Max-Bot-Api-Secret")), want) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false})
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	var update maxUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	// Max may send bot_started or a message with the /start payload
	payload := strings.TrimSpace(update.Payload)
	if payload == "" && update.Message != nil && update.Message.Body != nil {
		payload = strings.TrimPrefix(strings.TrimSpace(update.Message.Body.Text), "/start ")
		if payload == strings.TrimSpace(update.Message.Body.Text) {
			// not a /start command
			payload = update.Message.Body.Payload
		}
	}

	if update.UpdateType == "bot_started" || update.UpdateType == "message_created" {
		if payload != "" && update.ChatID != 0 {
			userID, valid := h.maxParseToken(payload)
			if valid {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = h.maxLinkAndGrantBonus(ctx, userID, update.ChatID)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// maxBonusMessages computes +10% of the daily limit (ceil, minimum 2) for one access.
func maxBonusMessages(dailyLimit int64) int64 {
	bonus := int64(math.Ceil(float64(dailyLimit) * 0.1))
	if bonus < 2 {
		bonus = 2
	}
	return bonus
}

// notificationBonusOverride is the bonus size configured in the admin UI
// (site_content key "notifications.bonus_messages", a number > 0). 0 = not set,
// the fallback formula applies.
func (h Handler) notificationBonusOverride(ctx context.Context) int64 {
	var raw []byte
	if err := h.DB.QueryRow(ctx,
		`SELECT value FROM site_content WHERE key = 'notifications.bonus_messages'`).Scan(&raw); err != nil {
		return 0
	}
	var n float64
	if json.Unmarshal(raw, &n) != nil || n <= 0 {
		return 0
	}
	return int64(n)
}

// grantDailyLimitBonus grants bonus messages for subscribing/binding.
// Size: the admin setting, otherwise the fallback formula, 10% of the MINIMUM
// current daily limit (ceil, minimum 2). The same amount
// is added to every active access; the amount itself is returned
// (for "+N messages"), not the sum over accesses; otherwise a user with
// ten accesses would be promised "+70" instead of "+7".
// Protection against double granting is up to the caller.
func (h Handler) grantDailyLimitBonus(ctx context.Context, userID int64) (int64, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT id, COALESCE(daily_message_limit, 10)
		FROM user_mode_access
		WHERE user_id = $1 AND active_to IS NOT NULL AND active_to > now()`,
		userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var accessIDs []int64
	minLimit := int64(-1)
	for rows.Next() {
		var id, limit int64
		if err := rows.Scan(&id, &limit); err != nil {
			return 0, err
		}
		accessIDs = append(accessIDs, id)
		if minLimit < 0 || limit < minLimit {
			minLimit = limit
		}
	}
	rows.Close()
	if len(accessIDs) == 0 {
		return 0, nil
	}

	bonus := h.notificationBonusOverride(ctx)
	if bonus <= 0 {
		bonus = maxBonusMessages(minLimit)
	}
	if _, err := h.DB.Exec(ctx, `
		UPDATE user_mode_access
		SET daily_message_limit = daily_message_limit + $2, updated_at = now()
		WHERE id = ANY($1::bigint[])`,
		accessIDs, bonus); err != nil {
		return 0, err
	}
	return bonus, nil
}

// maxLinkAndGrantBonus binds a Max chat_id to the user and on the first binding
// adds +10% messages (minimum 2) to the daily limit of every active access.
func (h Handler) maxLinkAndGrantBonus(ctx context.Context, userID, chatID int64) error {
	var alreadyGranted bool
	err := h.DB.QueryRow(ctx, `
		UPDATE users
		SET max_chat_id = $2, max_linked_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING max_bonus_granted`,
		userID, chatID).Scan(&alreadyGranted)
	if err != nil {
		return err
	}

	if !alreadyGranted {
		// Anti-farming for multi-binding: one Max chat can be bound to several
		// site accounts (that is normal), but only the first gets the bonus;
		// otherwise N registrations x one messenger = infinite bonuses.
		var linkedElsewhere bool
		_ = h.DB.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM users WHERE max_chat_id = $1 AND id <> $2 AND deleted_at IS NULL)`,
			chatID, userID).Scan(&linkedElsewhere)
		if linkedElsewhere {
			if _, err := h.DB.Exec(ctx, `
				UPDATE users SET max_bonus_granted = TRUE, updated_at = now() WHERE id = $1`,
				userID); err != nil {
				return err
			}
			if strings.TrimSpace(h.MaxBotToken) != "" {
				_ = h.sendMaxMessage(context.Background(), chatID, "✅ Уведомления подключены. Этот Max уже привязан к другому аккаунту, поэтому бонусные сообщения повторно не начисляются.")
			}
			return nil
		}
		totalBonus, err := h.grantDailyLimitBonus(ctx, userID)
		if err != nil {
			return err
		}
		_, err = h.DB.Exec(ctx, `
			UPDATE users SET max_bonus_granted = TRUE, updated_at = now() WHERE id = $1`,
			userID)
		if err != nil {
			return err
		}

		if strings.TrimSpace(h.MaxBotToken) != "" {
			msg := fmt.Sprintf("🎉 Уведомления подключены! +%d сообщений начислено на ваш аккаунт.", totalBonus)
			if totalBonus == 0 {
				msg = "🎉 Уведомления подключены! Дополнительные сообщения будут начислены при следующем доступе."
			}
			_ = h.sendMaxMessage(context.Background(), chatID, msg)
		}
	} else {
		if strings.TrimSpace(h.MaxBotToken) != "" {
			_ = h.sendMaxMessage(context.Background(), chatID, "✅ Уведомления уже были подключены ранее. Вы будете получать важные сообщения здесь.")
		}
	}
	return nil
}

// sendMaxMessage sends a message to the user in Max via the Bot API.
// MAX understands CommonMark ("format":"markdown": **bold**, _italic_,
// [link](url), verified live on botapi.max.ru). If the markup in the text
// is broken and the API returns an error, we retry as plain text so the notification
// gets through in any case.
func (h Handler) sendMaxMessage(ctx context.Context, chatID int64, text string) error {
	if strings.TrimSpace(h.MaxBotToken) == "" {
		return nil
	}
	if err := h.postMaxMessage(ctx, chatID, map[string]any{"text": text, "format": "markdown"}); err == nil {
		return nil
	}
	return h.postMaxMessage(ctx, chatID, map[string]any{"text": stripInlineMarkdown(text)})
}

func (h Handler) postMaxMessage(ctx context.Context, chatID int64, body map[string]any) error {
	// MAX Bot API: the recipient is set by the chat_id query parameter, the body is
	// just {"text": ...}. The {"recipient":{"chat_id":...}} format gives
	// "Unknown recipient" (verified on botapi.max.ru).
	payload, _ := json.Marshal(body)
	// The token goes in the Authorization header (without "Bearer"). The access_token
	// query parameter is declared deprecated ("use Authorization header").
	url := fmt.Sprintf("https://botapi.max.ru/messages?chat_id=%d", chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", h.MaxBotToken)
	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return redactSecretInError(err, h.MaxBotToken)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		// The token travels in the Authorization header; a proxy or error page
		// that echoes request headers must not put it into our logs.
		return redactSecretInError(fmt.Errorf("max sendMessage: HTTP %d: %s", resp.StatusCode, string(b)), h.MaxBotToken)
	}
	return nil
}
