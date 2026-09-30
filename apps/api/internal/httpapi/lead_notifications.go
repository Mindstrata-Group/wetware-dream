package httpapi

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

// Lead notifications: when a user in a mode with the trigger enabled
// has replied N times, the manager in Max receives one (per dialog) notification
// with a short AI summary of the conversation, the internal id and the user's promocode.
// Recipients and the threshold live in modes columns and are edited in the mode's admin page.

const (
	defaultLeadNotifyThreshold = 3
	leadSummaryHistoryLimit    = 40
	leadSummaryDialogMaxChars  = 8000
	defaultLeadSummaryPrompt   = `Ты помощник менеджера по продажам образовательного центра. По диалогу пользователя с ассистентом составь краткую выжимку для менеджера, 400-700 знаков, сплошным текстом без markdown и заголовков. Обязательно: кто человек и какая у него настоящая боль; на какой стадии осознанности он находится (по лестнице Ханта); метапрограммный профиль по речи (К/От, референция, масштаб); что уже пробовал; сомнения и опасения; каким следующим шагом менеджеру лучше выйти на контакт. Пиши только по фактам из диалога, ничего не выдумывай.`
)

// leadSummaryPrompt is configurable in the admin UI (see AdminLeadSummaryPrompt),
// like orchestration_prompt_default/dialog_summary_prompt_default.
func (h Handler) leadSummaryPrompt(ctx context.Context) string {
	prompt, err := h.systemSetting(ctx, "lead_summary_prompt_default")
	if err != nil || strings.TrimSpace(prompt) == "" {
		return defaultLeadSummaryPrompt
	}
	return strings.TrimSpace(prompt)
}

type leadNotifySettings struct {
	Enabled     bool
	ChatIDs     []int64
	TelegramIDs []string
	Threshold   int
}

// parseLeadChatIDs parses the recipient string ("111222333, 123") into Max chat
// ids; garbage and non-positive items are silently skipped.
func parseLeadChatIDs(s string) []int64 {
	var out []int64
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' }) {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// parseLeadTelegramIDs parses the Telegram recipient string ("123, @channel")
// into a list of chat_id/username; empty items are silently skipped.
func parseLeadTelegramIDs(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		id := strings.TrimSpace(part)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (h Handler) leadNotifySettingsForMode(ctx context.Context, modeID int64) (leadNotifySettings, error) {
	var enabled bool
	var chatIDs, telegramIDs string
	var threshold int
	err := h.DB.QueryRow(ctx, `
		select coalesce(lead_notify_enabled, false),
		       coalesce(lead_notify_chat_ids, ''),
		       coalesce(lead_notify_telegram_ids, ''),
		       coalesce(lead_notify_threshold, $2)
		from modes
		where id = $1`, modeID, defaultLeadNotifyThreshold).Scan(&enabled, &chatIDs, &telegramIDs, &threshold)
	if err != nil {
		return leadNotifySettings{}, err
	}
	if threshold < 1 {
		threshold = defaultLeadNotifyThreshold
	}
	return leadNotifySettings{Enabled: enabled, ChatIDs: parseLeadChatIDs(chatIDs), TelegramIDs: parseLeadTelegramIDs(telegramIDs), Threshold: threshold}, nil
}

// maybeSendLeadNotification is called after the assistant's reply (in a goroutine,
// with its own context): it checks the threshold, atomically marks the dialog as
// notified and sends the summary to managers. If no delivery succeeded,
// the mark is removed and the trigger fires again on the next message.
func (h Handler) maybeSendLeadNotification(ctx context.Context, userID, dialogID, modeID int64, modeName string) {
	settings, err := h.leadNotifySettingsForMode(ctx, modeID)
	if err != nil || !settings.Enabled || (len(settings.ChatIDs) == 0 && len(settings.TelegramIDs) == 0) {
		return
	}
	var userReplies int
	if err := h.DB.QueryRow(ctx, `select count(*) from dialogs_messages where dialog_id = $1 and role = 'user'`, dialogID).Scan(&userReplies); err != nil {
		log.Printf("lead notify: count messages dialog=%d: %v", dialogID, err)
		return
	}
	if userReplies < settings.Threshold {
		return
	}
	// Per-dialog dedup: only one goroutine gets through this UPDATE.
	cmd, err := h.DB.Exec(ctx, `update users_dialogs set lead_notified_at = now() where id = $1 and lead_notified_at is null`, dialogID)
	if err != nil || cmd.RowsAffected() == 0 {
		return
	}

	text := h.buildLeadNotificationText(ctx, userID, dialogID, modeName, userReplies)
	delivered := false
	for _, chatID := range settings.ChatIDs {
		if err := h.sendMaxMessage(ctx, chatID, text); err != nil {
			log.Printf("lead notify: send to max chat=%d dialog=%d: %v", chatID, dialogID, err)
			continue
		}
		delivered = true
	}
	for _, chatID := range settings.TelegramIDs {
		if err := h.sendTelegramMessage(ctx, chatID, text); err != nil {
			log.Printf("lead notify: send to telegram chat=%s dialog=%d: %v", chatID, dialogID, err)
			continue
		}
		delivered = true
	}
	if !delivered {
		// Delivered to nobody: remove the mark so the lead is not lost.
		_, _ = h.DB.Exec(ctx, `update users_dialogs set lead_notified_at = null where id = $1`, dialogID)
	}
}

func (h Handler) buildLeadNotificationText(ctx context.Context, userID, dialogID int64, modeName string, userReplies int) string {
	var email, phone string
	_ = h.DB.QueryRow(ctx, `select coalesce(email, ''), coalesce(phone, '') from users where id = $1`, userID).Scan(&email, &phone)
	var promocode string
	_ = h.DB.QueryRow(ctx, `
		select p.code
		from promocode_usages pu
		join promocodes p on p.id = pu.promocode_id
		where pu.user_id = $1
		order by pu.used_at desc
		limit 1`, userID).Scan(&promocode)

	var b strings.Builder
	b.WriteString("🔥 Тёплый лид в Стратуме\n")
	fmt.Fprintf(&b, "Пользователь: #%d", userID)
	if email != "" {
		b.WriteString(" · " + email)
	}
	if phone != "" {
		b.WriteString(" · " + phone)
	}
	b.WriteString("\n")
	if promocode != "" {
		fmt.Fprintf(&b, "Промокод: %s\n", promocode)
	}
	fmt.Fprintf(&b, "Режим: %s · ответов пользователя: %d\n\n", modeName, userReplies)
	b.WriteString(h.leadDialogSummary(ctx, userID, dialogID))
	return b.String()
}

// leadDialogSummary is a short AI summary of the dialog; if the AI fails it returns
// the user's last messages so the manager is not left without context.
func (h Handler) leadDialogSummary(ctx context.Context, userID, dialogID int64) string {
	messages, err := getDialogMessages(ctx, h.DB, dialogID, leadSummaryHistoryLimit, 0)
	if err != nil || len(messages) == 0 {
		return "Выжимка недоступна: не удалось прочитать диалог."
	}
	transcript := buildLeadTranscript(messages, leadSummaryDialogMaxChars)
	model, temperature := h.configuredAIModel(ctx, "ai_lead_summary_model", "ai_lead_summary_temperature", defaultAIOrchestrationModel, defaultAIOrchestrationTemperature)
	answer, _, _, _, err := h.doMechanicAIChat(ctx, "ai_lead_summary_provider", model, temperature, []map[string]string{
		{"role": "system", "content": h.leadSummaryPrompt(ctx)},
		{"role": "user", "content": transcript},
	}, buildXTitle(0, userID, "LEAD_SUMMARY"), openAIChatOptions{})
	if err != nil || strings.TrimSpace(answer) == "" {
		log.Printf("lead notify: summary AI failed dialog=%d: %v", dialogID, err)
		return "Выжимка (последние реплики пользователя):\n" + lastUserReplies(messages, 3)
	}
	return strings.TrimSpace(answer)
}

func buildLeadTranscript(messages []ChatMessage, maxChars int) string {
	var b strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			b.WriteString("Пользователь: ")
		case "assistant":
			b.WriteString("Ассистент: ")
		default:
			continue
		}
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}
	transcript := b.String()
	if len(transcript) > maxChars {
		// Trim from the start: the recent part of the conversation matters more for the summary.
		transcript = transcript[len(transcript)-maxChars:]
	}
	return transcript
}

func lastUserReplies(messages []ChatMessage, n int) string {
	var replies []string
	for i := len(messages) - 1; i >= 0 && len(replies) < n; i-- {
		if messages[i].Role == "user" {
			replies = append(replies, messages[i].Content)
		}
	}
	// restore chronological order
	for i, j := 0, len(replies)-1; i < j; i, j = i+1, j-1 {
		replies[i], replies[j] = replies[j], replies[i]
	}
	return strings.Join(replies, "\n---\n")
}
