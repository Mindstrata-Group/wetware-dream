package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type orchestrationCandidate struct {
	ID       int64
	Name     string
	Criteria string
}

type orchestrationDB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type orchestrationPromptPayload struct {
	Messages         []map[string]string
	AllowedModeIDs   []int64
	CandidateModeIDs []int64
	CandidateCount   int
	SystemPrompt     string
	UserPrompt       string
	LastUserMessage  string
}

type orchestrationDecisionLogEntry struct {
	UserID                  int64
	DialogID                int64
	CurrentModeID           int64
	SelectedModeID          int64
	AppliedModeID           int64
	Model                   string
	Temperature             float64
	UserMessagesSinceSwitch int
	CheckInterval           int
	CandidateModeIDs        []int64
	CandidateCount          int
	Decision                string
	Reason                  string
	RawAnswer               string
	RetryRawAnswer          string
	InputSystem             string
	InputUser               string
	LastUserMessage         string
	ResponseFormatUsed      bool
	RetryWithoutCurrent     bool
}

const (
	defaultOrchestrationHistoryLimit = 12
	minOrchestrationHistoryLimit     = 1
	maxOrchestrationHistoryLimit     = 50
)

const defaultOrchestrationPrompt = `Ты оркестратор режимов Стратума. Выбери один modeId из доступных режимов для следующего ответа.

Правила:
- Верни только JSON вида {"modeId":123,"reason":"коротко почему"}.
- Выбирай режим по задаче пользователя и criteria каждого режима, а не по отдельным словам-маркерам.
- Если текущий режим продолжает решать задачу не хуже остальных, верни текущий modeId.
- Не переключай режим ради просьбы "короче", "подробнее", "серьезнее", "проще", "пример", "продолжи" — это обычно изменение формы ответа, а не новая задача.
- Если пользователь явно просит другой взгляд: намерение, конфликт, переговоры, метафора, уточнить фразу, скрытый смысл, решение, причины — выбирай соответствующий режим.
- Последнее сообщение пользователя имеет больший вес, чем старый контекст диалога.
- Если старый контекст был про тревогу, но последнее сообщение говорит, что человек успокоился, просит идеи, результаты, усиление эффекта, решение или действие — не удерживай режим снижения тревоги только из-за старого контекста.
- Смотри на последние сообщения диалога: если человек уже работает в выбранной логике, не дергай переключение без явного нового намерения.`

func (h Handler) orchestrateMode(ctx context.Context, q orchestrationDB, userID, dialogID int64, current modeRow, knowledgeModeIDs []int64) (modeRow, bool, error) {
	ids, err := h.orchestrationModeIDs(ctx, q, userID, current.ID, knowledgeModeIDs)
	if err != nil {
		return current, false, err
	}
	if len(ids) < 2 {
		insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
			UserID:           userID,
			DialogID:         dialogID,
			CurrentModeID:    current.ID,
			AppliedModeID:    current.ID,
			CandidateModeIDs: ids,
			CandidateCount:   len(ids),
			Decision:         "skipped_insufficient_mode_ids",
		})
		return current, false, nil
	}
	if !idInList(current.ID, ids) {
		insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
			UserID:           userID,
			DialogID:         dialogID,
			CurrentModeID:    current.ID,
			AppliedModeID:    current.ID,
			CandidateModeIDs: ids,
			CandidateCount:   len(ids),
			Decision:         "skipped_current_not_candidate",
		})
		return current, false, nil
	}
	interval := current.OrchestratorCheckInterval
	if interval <= 0 {
		interval = 5
	}
	var userMessages int
	if err := q.QueryRow(ctx, `
		with last_switch as (
			select coalesce(max(id), 0) as message_id
			from dialogs_messages
			where dialog_id = $1
			  and role = 'system'
			  and content like $2
		)
		select count(*)
		from dialogs_messages dm
		cross join last_switch ls
		where dm.dialog_id = $1
		  and dm.role = 'user'
		  and dm.id > ls.message_id`, dialogID, modeSwitchTracePrefix+"%").Scan(&userMessages); err != nil {
		return current, false, err
	}
	if userMessages == 0 || userMessages%interval != 0 {
		insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
			UserID:                  userID,
			DialogID:                dialogID,
			CurrentModeID:           current.ID,
			AppliedModeID:           current.ID,
			UserMessagesSinceSwitch: userMessages,
			CheckInterval:           interval,
			CandidateModeIDs:        ids,
			CandidateCount:          len(ids),
			Decision:                "skipped_interval",
		})
		return current, false, nil
	}

	candidates, err := h.orchestrationCandidates(ctx, q, userID, ids)
	if err != nil || len(candidates) < 2 {
		insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
			UserID:                  userID,
			DialogID:                dialogID,
			CurrentModeID:           current.ID,
			AppliedModeID:           current.ID,
			UserMessagesSinceSwitch: userMessages,
			CheckInterval:           interval,
			CandidateModeIDs:        ids,
			CandidateCount:          len(candidates),
			Decision:                "skipped_insufficient_candidates",
		})
		return current, false, err
	}
	prompt, err := h.systemSetting(ctx, "orchestration_prompt_default")
	if err != nil || orchestrationPromptNeedsFallback(prompt) {
		prompt = defaultOrchestrationPrompt
	}
	recent, _ := getDialogMessages(ctx, q, dialogID, int64(h.orchestrationHistoryLimit(ctx)), 0)
	payload := buildOrchestrationPromptPayload(prompt, current, candidates, recent, false)
	orchestrationModel, orchestrationTemperature := h.configuredAIModel(ctx, "ai_orchestration_model", "ai_orchestration_temperature", defaultAIOrchestrationModel, defaultAIOrchestrationTemperature)
	answer, _, _, _, err := h.doMechanicAIChat(ctx, "ai_orchestration_provider", orchestrationModel, orchestrationTemperature, payload.Messages, buildXTitle(current.ID, userID, "ORCHESTRATION"), openAIChatOptions{
		ResponseFormat: orchestrationResponseFormat(payload.AllowedModeIDs),
	})
	if orchestrationResponseFormatUnsupported(err) {
		answer, _, _, _, err = h.doMechanicAIChat(ctx, "ai_orchestration_provider", orchestrationModel, orchestrationTemperature, payload.Messages, buildXTitle(current.ID, userID, "ORCHESTRATION"), openAIChatOptions{})
	}
	if err != nil {
		insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
			UserID:                  userID,
			DialogID:                dialogID,
			CurrentModeID:           current.ID,
			AppliedModeID:           current.ID,
			Model:                   orchestrationModel,
			Temperature:             orchestrationTemperature,
			UserMessagesSinceSwitch: userMessages,
			CheckInterval:           interval,
			CandidateModeIDs:        payload.CandidateModeIDs,
			CandidateCount:          payload.CandidateCount,
			Decision:                "ai_error",
			Reason:                  err.Error(),
			InputSystem:             payload.SystemPrompt,
			InputUser:               payload.UserPrompt,
			LastUserMessage:         payload.LastUserMessage,
			ResponseFormatUsed:      true,
		})
		return current, false, err
	}
	selectedID, reason := parseOrchestratorDecision(answer, payload.AllowedModeIDs)
	if selectedID == 0 || selectedID == current.ID {
		decision := "invalid_response"
		if selectedID == current.ID {
			if h.previousOrchestrationKeptCurrent(ctx, q, dialogID, current.ID) {
				next, switched, retryAnswer, retryReason, retryErr := h.forceAlternativeAfterRepeatedKeep(ctx, q, userID, current, candidates, recent, prompt, orchestrationModel, orchestrationTemperature)
				if retryErr != nil {
					insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
						UserID:                  userID,
						DialogID:                dialogID,
						CurrentModeID:           current.ID,
						SelectedModeID:          current.ID,
						AppliedModeID:           current.ID,
						Model:                   orchestrationModel,
						Temperature:             orchestrationTemperature,
						UserMessagesSinceSwitch: userMessages,
						CheckInterval:           interval,
						CandidateModeIDs:        payload.CandidateModeIDs,
						CandidateCount:          payload.CandidateCount,
						Decision:                "forced_error",
						Reason:                  retryErr.Error(),
						RawAnswer:               answer,
						RetryRawAnswer:          retryAnswer,
						InputSystem:             payload.SystemPrompt,
						InputUser:               payload.UserPrompt,
						LastUserMessage:         payload.LastUserMessage,
						ResponseFormatUsed:      true,
						RetryWithoutCurrent:     true,
					})
					return current, false, retryErr
				}
				logEntry := orchestrationDecisionLogEntry{
					UserID:                  userID,
					DialogID:                dialogID,
					CurrentModeID:           current.ID,
					SelectedModeID:          next.ID,
					AppliedModeID:           current.ID,
					Model:                   orchestrationModel,
					Temperature:             orchestrationTemperature,
					UserMessagesSinceSwitch: userMessages,
					CheckInterval:           interval,
					CandidateModeIDs:        payload.CandidateModeIDs,
					CandidateCount:          payload.CandidateCount,
					Decision:                "forced_invalid_response",
					Reason:                  retryReason,
					RawAnswer:               answer,
					RetryRawAnswer:          retryAnswer,
					InputSystem:             payload.SystemPrompt,
					InputUser:               payload.UserPrompt,
					LastUserMessage:         payload.LastUserMessage,
					ResponseFormatUsed:      true,
					RetryWithoutCurrent:     true,
				}
				if switched {
					logEntry.AppliedModeID = next.ID
					logEntry.Decision = "forced_switch_after_repeat_keep"
					insertOrchestrationDecisionLog(ctx, q, logEntry)
					return next, true, nil
				}
				insertOrchestrationDecisionLog(ctx, q, logEntry)
				return current, false, nil
			}
			decision = "kept_current"
		}
		insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
			UserID:                  userID,
			DialogID:                dialogID,
			CurrentModeID:           current.ID,
			SelectedModeID:          selectedID,
			AppliedModeID:           current.ID,
			Model:                   orchestrationModel,
			Temperature:             orchestrationTemperature,
			UserMessagesSinceSwitch: userMessages,
			CheckInterval:           interval,
			CandidateModeIDs:        payload.CandidateModeIDs,
			CandidateCount:          payload.CandidateCount,
			Decision:                decision,
			Reason:                  reason,
			RawAnswer:               answer,
			InputSystem:             payload.SystemPrompt,
			InputUser:               payload.UserPrompt,
			LastUserMessage:         payload.LastUserMessage,
			ResponseFormatUsed:      true,
		})
		return current, false, nil
	}
	next, err := getUserAccessibleModeByID(ctx, q, userID, selectedID)
	if err != nil {
		return current, false, err
	}
	insertOrchestrationDecisionLog(ctx, q, orchestrationDecisionLogEntry{
		UserID:                  userID,
		DialogID:                dialogID,
		CurrentModeID:           current.ID,
		SelectedModeID:          selectedID,
		AppliedModeID:           next.ID,
		Model:                   orchestrationModel,
		Temperature:             orchestrationTemperature,
		UserMessagesSinceSwitch: userMessages,
		CheckInterval:           interval,
		CandidateModeIDs:        payload.CandidateModeIDs,
		CandidateCount:          payload.CandidateCount,
		Decision:                "switched",
		Reason:                  reason,
		RawAnswer:               answer,
		InputSystem:             payload.SystemPrompt,
		InputUser:               payload.UserPrompt,
		LastUserMessage:         payload.LastUserMessage,
		ResponseFormatUsed:      true,
	})
	return next, true, nil
}

func (h Handler) orchestrationHistoryLimit(ctx context.Context) int {
	return intSetting(ctx, h, "ai_orchestration_history_limit", defaultOrchestrationHistoryLimit, minOrchestrationHistoryLimit, maxOrchestrationHistoryLimit)
}

func buildOrchestrationPromptPayload(prompt string, current modeRow, candidates []orchestrationCandidate, recent []ChatMessage, forceAlternative bool) orchestrationPromptPayload {
	candidateText := new(strings.Builder)
	allowedCandidateIDs := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		allowedCandidateIDs = append(allowedCandidateIDs, c.ID)
		_, _ = fmt.Fprintf(candidateText, "modeId=%d; name=%s; criteria=%s\n", c.ID, c.Name, c.Criteria)
	}
	lastUser := lastUserMessageForOrchestration(recent)
	if strings.TrimSpace(lastUser) == "" {
		lastUser = "(нет последнего сообщения пользователя)"
	}
	transcript := new(strings.Builder)
	for _, msg := range recent {
		if msg.Role == "summary" || isModeSwitchTraceMessage(msg) {
			continue
		}
		_, _ = fmt.Fprintf(transcript, "%s: %s\n", msg.Role, msg.Content)
	}
	instruction := "Выбери modeId для следующего ответа. Если текущий подходит лучше всего, верни его."
	if forceAlternative {
		instruction = fmt.Sprintf("Предыдущая проверка уже оставила текущий modeId=%d. В этой проверке не выбирай текущий режим; выбери лучший альтернативный modeId из доступного списка.", current.ID)
	}
	systemPrompt := buildRuntimeModePrompt(prompt, "")
	userPrompt := fmt.Sprintf("Текущий modeId=%d (%s).\n\nПоследнее сообщение пользователя (главный вес для следующего ответа):\nuser: %s\n\nДоступные режимы:\n%s\nНедавний диалог (контекст, меньший вес чем последнее сообщение):\n%s\n%s", current.ID, current.Name, lastUser, candidateText.String(), transcript.String(), instruction)
	return orchestrationPromptPayload{
		Messages: []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		AllowedModeIDs:   allowedCandidateIDs,
		CandidateModeIDs: allowedCandidateIDs,
		CandidateCount:   len(candidates),
		SystemPrompt:     systemPrompt,
		UserPrompt:       userPrompt,
		LastUserMessage:  lastUser,
	}
}

func lastUserMessageForOrchestration(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func (h Handler) forceAlternativeAfterRepeatedKeep(ctx context.Context, q orchestrationDB, userID int64, current modeRow, candidates []orchestrationCandidate, recent []ChatMessage, prompt, model string, temperature float64) (modeRow, bool, string, string, error) {
	alternatives := make([]orchestrationCandidate, 0, len(candidates)-1)
	for _, candidate := range candidates {
		if candidate.ID != current.ID {
			alternatives = append(alternatives, candidate)
		}
	}
	if len(alternatives) == 0 {
		return current, false, "", "нет альтернативного режима", nil
	}
	payload := buildOrchestrationPromptPayload(prompt, current, alternatives, recent, true)
	answer, _, _, _, err := h.doMechanicAIChat(ctx, "ai_orchestration_provider", model, temperature, payload.Messages, buildXTitle(current.ID, userID, "ORCHESTRATION_RETRY"), openAIChatOptions{
		ResponseFormat: orchestrationResponseFormat(payload.AllowedModeIDs),
	})
	if orchestrationResponseFormatUnsupported(err) {
		answer, _, _, _, err = h.doMechanicAIChat(ctx, "ai_orchestration_provider", model, temperature, payload.Messages, buildXTitle(current.ID, userID, "ORCHESTRATION_RETRY"), openAIChatOptions{})
	}
	if err != nil {
		return current, false, answer, "", err
	}
	selectedID, reason := parseOrchestratorDecision(answer, payload.AllowedModeIDs)
	if selectedID == 0 || selectedID == current.ID {
		return current, false, answer, reason, nil
	}
	next, err := getUserAccessibleModeByID(ctx, q, userID, selectedID)
	if err != nil {
		return current, false, answer, reason, err
	}
	return next, true, answer, reason, nil
}

func (h Handler) previousOrchestrationKeptCurrent(ctx context.Context, q orchestrationDB, dialogID, currentModeID int64) bool {
	var selectedModeID int64
	var decision string
	err := q.QueryRow(ctx, `
		select coalesce(selected_mode_id, 0), decision
		from orchestration_decision_logs
		where dialog_id = $1
		  and decision in ('kept_current', 'switched', 'forced_switch_after_repeat_keep', 'forced_invalid_response', 'invalid_response', 'forced_error', 'ai_error')
		order by id desc
		limit 1`, dialogID).Scan(&selectedModeID, &decision)
	if err != nil {
		return false
	}
	return decision == "kept_current" && selectedModeID == currentModeID
}

func insertOrchestrationDecisionLog(ctx context.Context, q orchestrationDB, entry orchestrationDecisionLogEntry) {
	if entry.Decision == "" {
		entry.Decision = "unknown"
	}
	_, _ = q.Exec(ctx, `
		delete from orchestration_decision_logs
		where id in (
			select id
			from orchestration_decision_logs
			where created_at < now() - interval '7 days'
			order by id asc
			limit 1000
		)`)
	_, _ = q.Exec(ctx, `
		delete from orchestration_decision_logs
		where id in (
			select id
			from orchestration_decision_logs
			where id not in (
				select id
				from orchestration_decision_logs
				order by id desc
				limit 4999
			)
			order by id asc
			limit 1000
		)`)
	_, _ = q.Exec(ctx, `
		insert into orchestration_decision_logs (
			user_id, dialog_id, current_mode_id, selected_mode_id, applied_mode_id,
			model, temperature, user_messages_since_switch, check_interval,
			candidate_mode_ids, candidate_count, decision, reason, raw_answer, retry_raw_answer,
			input_system, input_user, last_user_message, response_format_used, retry_without_current
		) values (
			$1, $2, $3, nullif($4, 0), nullif($5, 0),
			$6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20
		)`,
		entry.UserID,
		entry.DialogID,
		entry.CurrentModeID,
		entry.SelectedModeID,
		entry.AppliedModeID,
		entry.Model,
		entry.Temperature,
		entry.UserMessagesSinceSwitch,
		entry.CheckInterval,
		entry.CandidateModeIDs,
		entry.CandidateCount,
		entry.Decision,
		entry.Reason,
		entry.RawAnswer,
		entry.RetryRawAnswer,
		entry.InputSystem,
		entry.InputUser,
		entry.LastUserMessage,
		entry.ResponseFormatUsed,
		entry.RetryWithoutCurrent,
	)
}

func (h Handler) orchestrationModeIDs(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, userID, currentModeID int64, knowledgeModeIDs []int64) ([]int64, error) {
	ids := uniquePositiveIDs(knowledgeModeIDs)
	if len(ids) >= 2 {
		return ids, nil
	}
	promoIDs, err := activePromocodeModeIDsForCurrentMode(ctx, q, userID, currentModeID)
	if err != nil || len(promoIDs) < 2 {
		return ids, err
	}
	return promoIDs, nil
}

func activePromocodeModeIDsForCurrentMode(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, userID, currentModeID int64) ([]int64, error) {
	rows, err := q.Query(ctx, `
		with current_source as (
			select uma.source_id
			from user_mode_access uma
			where uma.user_id = $1
			  and uma.mode_id = $2
			  and uma.access_type = 'promocode'
			  and uma.source_id is not null
			  and (uma.active_from is null or uma.active_from <= now())
			  and (uma.active_to is null or uma.active_to >= now())
			order by coalesce(uma.priority, 0) desc, (uma.active_to is null) desc, uma.active_to desc nulls first, uma.id desc
			limit 1
		)
		select distinct uma.mode_id
		from user_mode_access uma
		join current_source cs on cs.source_id = uma.source_id
		join modes m on m.id = uma.mode_id
		where uma.user_id = $1
		  and uma.access_type = 'promocode'
		  and (uma.active_from is null or uma.active_from <= now())
		  and (uma.active_to is null or uma.active_to >= now())
		  and m.hidden_at is null
		order by uma.mode_id asc`, userID, currentModeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func orchestrationPromptNeedsFallback(prompt string) bool {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	return !strings.Contains(lower, "json") || !strings.Contains(trimmed, "modeId")
}

func orchestrationResponseFormat(allowed []int64) map[string]any {
	enum := make([]any, 0, len(allowed))
	for _, id := range allowed {
		enum = append(enum, id)
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "mindstrata_mode_orchestration",
			"strict": true,
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"modeId": map[string]any{
						"type": "integer",
						"enum": enum,
					},
					"reason": map[string]any{
						"type": "string",
					},
				},
				"required":             []string{"modeId", "reason"},
				"additionalProperties": false,
			},
		},
	}
}

func orchestrationResponseFormatUnsupported(err error) bool {
	if err == nil {
		return false
	}
	debug, ok := liveAIDebugFromError(err)
	if !ok || (debug.Status != http.StatusBadRequest && debug.Status != http.StatusUnprocessableEntity) {
		return false
	}
	body := strings.ToLower(debug.ResponseBody)
	return strings.Contains(body, "response_format") || strings.Contains(body, "json_schema") || strings.Contains(body, "structured") || strings.Contains(body, "schema")
}

func (h Handler) orchestrationCandidates(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, userID int64, ids []int64) ([]orchestrationCandidate, error) {
	rows, err := q.Query(ctx, `
		select m.id, m.name, coalesce(m.criteria, '')
		from modes m
		where m.id = any($1::bigint[])
		  and m.hidden_at is null
		  and exists (
			select 1 from user_mode_access uma
			where uma.user_id = $2
			  and uma.mode_id = m.id
			  and (uma.active_from is null or uma.active_from <= now())
			  and (uma.active_to is null or uma.active_to >= now())
		  )
		order by m.id asc`, ids, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []orchestrationCandidate{}
	for rows.Next() {
		var c orchestrationCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Criteria); err != nil {
			return nil, err
		}
		quota, err := h.dailyQuota(ctx, userID, c.ID)
		if err != nil {
			return nil, err
		}
		if quota != nil && quota.Remaining != nil && *quota.Remaining <= 0 {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func parseOrchestratorDecision(answer string, allowed []int64) (int64, string) {
	var payload struct {
		ModeID int64  `json:"modeId"`
		Reason string `json:"reason"`
	}
	trimmed := strings.TrimSpace(answer)
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end >= start {
			_ = json.Unmarshal([]byte(trimmed[start:end+1]), &payload)
		}
	}
	if idInList(payload.ModeID, allowed) {
		return payload.ModeID, strings.TrimSpace(payload.Reason)
	}
	for _, id := range allowed {
		if strings.Contains(trimmed, strconv.FormatInt(id, 10)) {
			return id, strings.TrimSpace(payload.Reason)
		}
	}
	return 0, strings.TrimSpace(payload.Reason)
}

func parseOrchestratorModeID(answer string, allowed []int64) int64 {
	modeID, _ := parseOrchestratorDecision(answer, allowed)
	return modeID
}

func idInList(id int64, ids []int64) bool {
	for _, item := range ids {
		if item == id {
			return true
		}
	}
	return false
}
