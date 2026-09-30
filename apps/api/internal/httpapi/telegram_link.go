package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// tgUserToken is the Telegram binding HMAC token, similar to maxUserToken
// but signed with the TG bot token. Payload format: "{userID}_{hmac[:16]}".
func (h Handler) tgUserToken(userID int64) string {
	mac := hmac.New(sha256.New, []byte("tg:"+h.TelegramBotToken))
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(userID))
	mac.Write(b)
	sum := fmt.Sprintf("%x", mac.Sum(nil))
	return fmt.Sprintf("%d_%s", userID, sum[:16])
}

func (h Handler) tgParseToken(token string) (int64, bool) {
	parts := strings.SplitN(token, "_", 2)
	if len(parts) != 2 {
		return 0, false
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || userID <= 0 {
		return 0, false
	}
	expected := strings.SplitN(h.tgUserToken(userID), "_", 2)
	if len(expected) != 2 {
		return 0, false
	}
	return userID, hmac.Equal([]byte(parts[1]), []byte(expected[1]))
}

// tgBotStartLink is the deeplink https://t.me/<bot>?start=<token> (the official
// Telegram format; payload up to 64 characters, base64url alphabet).
func (h Handler) tgBotStartLink(userID int64) string {
	if strings.TrimSpace(h.TelegramBotUsername) == "" {
		return ""
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", h.TelegramBotUsername, h.tgUserToken(userID))
}

// TelegramNotificationsStartLink handles GET /api/notifications/telegram/start-link.
// Returns the deeplink and the binding status. No webhook is set for the TG bot
// (incoming traffic from Telegram to a server in Russia is unreliable due to blocking), so
// on every frontend poll we process fresh getUpdates through the proxy and
// match "/start <payload>".
func (h Handler) TelegramNotificationsStartLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}

	h.processTelegramLinkUpdates(r.Context())

	var telegramID *int64
	_ = h.DB.QueryRow(r.Context(), `SELECT telegram_id FROM users WHERE id = $1 AND deleted_at IS NULL`, user.ID).Scan(&telegramID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"startLink":      h.tgBotStartLink(user.ID),
		"botUsername":    h.TelegramBotUsername,
		"telegramLinked": telegramID != nil,
	})
}

// TelegramNotificationsUnlink — POST /api/notifications/telegram/unlink
func (h Handler) TelegramNotificationsUnlink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	// As with Max: unbinding is forbidden for the first day after binding.
	var linkedAt *time.Time
	_ = h.DB.QueryRow(r.Context(), `SELECT telegram_linked_at FROM users WHERE id = $1 AND deleted_at IS NULL`, user.ID).Scan(&linkedAt)
	if linkedAt != nil {
		if elapsed := time.Since(*linkedAt); elapsed < maxUnlinkCooldown {
			hoursLeft := int((maxUnlinkCooldown - elapsed).Hours()) + 1
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": false, "code": "cooldown", "hoursLeft": hoursLeft,
				"error": "Отвязать Telegram можно только через сутки после подключения.",
			})
			return
		}
	}
	if _, err := h.DB.Exec(r.Context(), `UPDATE users SET telegram_id = NULL, telegram_linked_at = NULL, updated_at = now() WHERE id = $1`, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "telegramLinked": false})
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
		Chat *struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

// processTelegramLinkUpdates fetches the bot's fresh updates and binds
// accounts by "/start <payload>". An advisory lock rules out parallel
// processing (polls from different users); acknowledging the offset deletes
// processed updates on the Telegram side.
func (h Handler) processTelegramLinkUpdates(ctx context.Context) {
	if strings.TrimSpace(h.TelegramBotToken) == "" || h.DB == nil {
		return
	}
	conn, err := h.DB.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('tg_link_updates'))`).Scan(&locked); err != nil || !locked {
		return
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('tg_link_updates'))`)

	client := h.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(fmt.Sprintf("%s/bot%s/getUpdates?timeout=0&allowed_updates=%%5B%%22message%%22%%5D", h.telegramAPIBase(), h.TelegramBotToken))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var payload struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || !payload.OK || len(payload.Result) == 0 {
		return
	}

	var lastID int64
	for _, upd := range payload.Result {
		if upd.UpdateID > lastID {
			lastID = upd.UpdateID
		}
		if upd.Message == nil || upd.Message.Chat == nil {
			continue
		}
		text := strings.TrimSpace(upd.Message.Text)
		if !strings.HasPrefix(text, "/start ") {
			continue
		}
		userID, valid := h.tgParseToken(strings.TrimSpace(strings.TrimPrefix(text, "/start ")))
		if !valid {
			continue
		}
		linkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		h.tgLinkAndGrantBonus(linkCtx, userID, upd.Message.Chat.ID, func() string {
			if upd.Message.From != nil {
				return upd.Message.From.Username
			}
			return ""
		}())
		cancel()
	}
	// Acknowledge the offset: Telegram forgets the processed updates.
	if lastID > 0 {
		confirm, err := client.Get(fmt.Sprintf("%s/bot%s/getUpdates?timeout=0&limit=1&offset=%d", h.telegramAPIBase(), h.TelegramBotToken, lastID+1))
		if err == nil {
			confirm.Body.Close()
		}
	}
}

// tgLinkAndGrantBonus binds Telegram and grants the bonus once.
// Multi-binding is allowed; the bonus goes only to the first account with this chat_id
// (anti-farming) and once per user (the consent_bonuses ledger).
func (h Handler) tgLinkAndGrantBonus(ctx context.Context, userID, chatID int64, username string) {
	if _, err := h.DB.Exec(ctx, `
		UPDATE users SET telegram_id = $2, telegram_username = COALESCE(NULLIF($3, ''), telegram_username),
		       telegram_linked_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, userID, chatID, username); err != nil {
		return
	}

	var linkedElsewhere bool
	_ = h.DB.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE telegram_id = $1 AND id <> $2 AND deleted_at IS NULL)`,
		chatID, userID).Scan(&linkedElsewhere)

	var firstTime bool
	err := h.DB.QueryRow(ctx, `
		INSERT INTO notification_consent_bonuses (user_id, channel, consent_type)
		VALUES ($1, 'telegram', 'link')
		ON CONFLICT (user_id, channel, consent_type) DO NOTHING
		RETURNING true`, userID).Scan(&firstTime)

	msg := "✅ Уведомления в Telegram подключены."
	if err == nil && firstTime && !linkedElsewhere {
		if bonus, gerr := h.grantDailyLimitBonus(ctx, userID); gerr == nil && bonus > 0 {
			_, _ = h.DB.Exec(ctx, `
				UPDATE notification_consent_bonuses SET bonus_messages = $2
				WHERE user_id = $1 AND channel = 'telegram' AND consent_type = 'link'`, userID, bonus)
			msg = fmt.Sprintf("🎉 Уведомления в Telegram подключены! +%d сообщений начислено на ваш аккаунт.", bonus)
		}
	} else if linkedElsewhere {
		msg = "✅ Уведомления в Telegram подключены. Этот Telegram уже привязан к другому аккаунту, бонус повторно не начисляется."
	}
	sendCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = h.sendTelegramMessage(sendCtx, strconv.FormatInt(chatID, 10), msg)
}
