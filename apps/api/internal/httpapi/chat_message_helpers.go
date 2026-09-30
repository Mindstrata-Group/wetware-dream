package httpapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	chatapi "mindstrata-stage1/api/internal/httpapi/chat"
)

const modeSwitchTracePrefix = "[mode-switch] "

func getDialogModeByID(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID, dialogID int64) (modeRow, bool, error) {
	var m modeRow
	var hasActiveAccess bool
	err := q.QueryRow(ctx, `
		select
			coalesce(m.id, d.mode_id),
			coalesce(m.name, 'Базовый ИИ'),
			coalesce(m.prompt, ''),
			coalesce(m.welcome_message, ''),
			coalesce(m.ai_model, 'openai/gpt-4o-mini'),
			coalesce(m.ai_provider, 'vsegpt'),
			coalesce(m.thinking_mode, 'default'),
			coalesce(m.model_temperature, 0.7),
			coalesce(m.criteria, ''),
			coalesce(m.orchestrator_check_interval, 5),
			coalesce(m.ai_max_tokens, 0),
			(m.id is not null and m.hidden_at is null and exists (
				select 1
				from user_mode_access uma
				where uma.user_id = d.user_id
				  and uma.mode_id = m.id
				  and (uma.active_from is null or uma.active_from <= now())
				  and (uma.active_to is null or uma.active_to >= now())
			)) as has_active_access
		from users_dialogs d
		left join modes m on m.id = d.mode_id
		where d.id = $1
		  and d.user_id = $2
		  and d.deleted_at is null`,
		dialogID,
		userID,
	).Scan(&m.ID, &m.Name, &m.Prompt, &m.WelcomeMessage, &m.AIModel, &m.AIProvider, &m.ThinkingMode, &m.Temperature, &m.Criteria, &m.OrchestratorCheckInterval, &m.MaxTokens, &hasActiveAccess)

	return m, hasActiveAccess, err
}

func getModeByDialogID(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID, dialogID int64) (modeRow, error) {
	mode, hasActiveAccess, err := getDialogModeByID(ctx, q, userID, dialogID)
	if err != nil {
		return modeRow{}, err
	}
	if !hasActiveAccess {
		return modeRow{}, pgx.ErrNoRows
	}
	return mode, nil
}

func insertDialogMessage(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, dialogID int64, role, content string) (ChatMessage, error) {
	var msg ChatMessage
	err := q.QueryRow(ctx, `insert into dialogs_messages (dialog_id, role, content) values ($1,$2,$3) returning id, role, content, created_at`, dialogID, role, content).Scan(&msg.ID, &msg.Role, &msg.Content, &msg.CreatedAt)
	return msg, err
}

func insertModeSwitchTrace(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, dialogID int64, fromModeName, toModeName string, automatic bool) (*ChatMessage, error) {
	content := modeSwitchTraceContent(fromModeName, toModeName, automatic)
	msg, err := insertDialogMessage(ctx, q, dialogID, "system", content)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func modeSwitchTraceContent(fromModeName, toModeName string, automatic bool) string {
	fromModeName = strings.TrimSpace(fromModeName)
	toModeName = strings.TrimSpace(toModeName)
	if fromModeName == "" {
		fromModeName = "Базовый ИИ"
	}
	if toModeName == "" {
		toModeName = "Базовый ИИ"
	}
	if automatic {
		return modeSwitchTracePrefix + fmt.Sprintf("Стратум переключил режим: «%s» → «%s».", fromModeName, toModeName)
	}
	return modeSwitchTracePrefix + fmt.Sprintf("Режим переключён вручную: «%s» → «%s».", fromModeName, toModeName)
}

func isModeSwitchTraceMessage(msg ChatMessage) bool {
	return msg.Role == "system" && strings.HasPrefix(strings.TrimSpace(msg.Content), modeSwitchTracePrefix)
}

func getDialogMessages(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, dialogID int64, limit int64, beforeID int64) ([]ChatMessage, error) {
	sql := `
		select dm.id, dm.role, dm.content, dm.created_at, dmau.mode_id, coalesce(m.name, '')
		from dialogs_messages dm
		left join lateral (
			select mode_id
			from dialog_message_access_usage
			where dialog_message_id = dm.id
			order by created_at desc, id desc
			limit 1
		) dmau on true
		left join modes m on m.id = dmau.mode_id
		where dm.dialog_id=$1`
	args := []any{dialogID}
	if beforeID > 0 {
		sql += ` and dm.id < $2`
		args = append(args, beforeID)
	}
	sql += ` order by dm.created_at desc, dm.id desc limit $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reversed []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.CreatedAt, &msg.ModeID, &msg.ModeName); err != nil {
			return nil, err
		}
		reversed = append(reversed, msg)
	}
	// reverse to chronological order
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, rows.Err()
}

func normalizeResponseMode(v string) string {
	return chatapi.NormalizeResponseMode(v)
}

func slugModeName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "\\", "_")

	var b strings.Builder
	for _, r := range v {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
		case r >= 'а' && r <= 'я':
			b.WriteRune(r)
		case r == 'ё':
			b.WriteRune(r)
		}
	}

	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "mode"
	}
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func buildXTitle(modeID, userID int64, suffix string) string {
	return fmt.Sprintf("Mindstrata_USER_%d_MODE_%d_%s", userID, modeID, suffix)
}

func sanitizeXTitle(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		out = "Mindstrata"
	}
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func buildTestSummary(modeName string, dialogID int64, messages []ChatMessage) string {
	lastUser := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUser = messages[i].Content
			break
		}
	}
	return fmt.Sprintf("[TEST SUMMARY]\nРежим: %s\nDialog ID: %d\nСообщений в сессии: %d\nПоследний запрос: %q\n\nСледующий шаг: продолжить диалог, проверить другой режим или перейти к реальному AI.", modeName, dialogID, len(messages), lastUser)
}
