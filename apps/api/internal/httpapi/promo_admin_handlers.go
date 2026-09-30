package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const promoAdminStatusCacheTTL = 60 * time.Second

func (h Handler) PromoAdminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("promo")))
	if cached, ok := h.c.adminPromoAdmin.get(code); ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cached)
		return
	}
	promo, ok := h.loadPromoAdminContext(w, r, code, true)
	if !ok {
		return
	}

	// All 5 independent queries run in parallel.
	type statsRes struct {
		msgCount    int64
		activations int64
		err         error
	}
	chPrompts := make(chan []map[string]any, 1)
	chModes := make(chan []map[string]any, 1)
	chUsage := make(chan [2]int64, 1) // [used, remaining(-1=nil)]
	chStats := make(chan statsRes, 1)
	chHistory := make(chan []map[string]any, 1)

	ctx := r.Context()
	go func() { v, _ := h.listSummaryPrompts(ctx); chPrompts <- v }()
	go func() { v, _ := h.promoAdminModes(ctx, promo.id); chModes <- v }()
	go func() {
		used, rem := h.promoSummaryUsage(ctx, promo.id, promo.summaryLimit)
		var remVal int64 = -1
		if rem != nil {
			remVal = *rem
		}
		chUsage <- [2]int64{used, remVal}
	}()
	go func() {
		mc, ac, err := h.promoAdminMessageStats(ctx, promo.id)
		chStats <- statsRes{mc, ac, err}
	}()
	go func() { v, _ := h.promoAdminSummaryHistory(ctx, promo.id, 20); chHistory <- v }()

	prompts := <-chPrompts
	modes := <-chModes
	usage := <-chUsage
	stats := <-chStats
	history := <-chHistory

	if stats.err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": stats.err.Error()})
		return
	}

	used := usage[0]
	var remaining *int64
	if usage[1] >= 0 {
		rv := usage[1]
		remaining = &rv
	}

	promoMap := promo.toMap()
	promoMap["summaryUsed"] = used
	promoMap["summaryRemaining"] = remaining
	promoMap["messageCount"] = stats.msgCount
	promoMap["activationsWithoutMessages"] = stats.activations
	body, _ := json.Marshal(map[string]any{"ok": true, "promo": promoMap, "prompts": prompts, "modes": modes, "history": history})
	h.c.adminPromoAdmin.set(code, body, promoAdminStatusCacheTTL)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (h Handler) PromoAdminSummarize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Code     string  `json:"code"`
		Key      string  `json:"key"`
		ModeIDs  []int64 `json:"modeIds"`
		PromptID int64   `json:"promptId"`
	}
	if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("key")) == "" {
		r.URL.RawQuery = strings.TrimLeft(r.URL.RawQuery+"&key="+req.Key, "&")
	}
	promo, ok := h.loadPromoAdminContext(w, r, strings.ToUpper(strings.TrimSpace(req.Code)), true)
	if !ok {
		return
	}
	if !promoAdminSummaryWindowActive(promo.activeFrom, promo.activeTo) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Окно резюмирования по промокоду не активно"})
		return
	}
	prompt := ""
	if req.PromptID > 0 {
		_ = h.DB.QueryRow(r.Context(), `select prompt from admin_summary_prompts where id=$1`, req.PromptID).Scan(&prompt)
	}
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "prompt is required"})
		return
	}
	used, remaining := h.promoSummaryUsage(r.Context(), promo.id, promo.summaryLimit)
	if remaining != nil && *remaining <= 0 {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Лимит резюмирований закончился", "used": used, "remaining": remaining})
		return
	}
	modeIDs := uniquePositiveIDs(req.ModeIDs)
	items, exportText, err := h.collectAdminExport(r.Context(), modeIDs, nil, []int64{promo.id}, "all", 10000, "", "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Нет сообщений для резюмирования", "messageCount": 0})
		return
	}
	model, temperature := h.adminSummaryModel(r.Context())
	messages := []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": "Задача: проанализировать сырые сообщения пользователей по промокоду и выдать нумерованное саммари.\n\n" + exportText}}
	result, _, _, _, err := h.doMechanicAIChat(r.Context(), "ai_summary_provider", model, temperature, messages, buildXTitle(0, 0, "PROMO_SUMMARY"), openAIChatOptions{})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":           false,
			"error":        "summary unavailable",
			"code":         "live_ai_error",
			"messageCount": len(items),
			"sourceBytes":  len(exportText),
			"approxTokens": approxTokenCount(exportText),
		})
		return
	}
	filters := map[string]any{"modeIds": modeIDs, "promocodeIds": []int64{promo.id}}
	filterJSON, _ := json.Marshal(filters)
	var id int64
	_ = h.DB.QueryRow(r.Context(), `insert into admin_export_summaries (promocode_id, prompt_id, prompt, filters, source_message_count, source_bytes, approx_tokens, result, created_at) values ($1,$2,$3,$4::jsonb,$5,$6,$7,$8,now()) returning id`, promo.id, nullablePositive(req.PromptID), prompt, string(filterJSON), len(items), len(exportText), approxTokenCount(exportText), result).Scan(&id)
	used, remaining = h.promoSummaryUsage(r.Context(), promo.id, promo.summaryLimit)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "summaryId": id, "result": result, "messageCount": len(items), "sourceBytes": len(exportText), "approxTokens": approxTokenCount(exportText), "used": used, "remaining": remaining, "createdAt": time.Now()})
}

type promoAdminContext struct {
	id           int64
	code         string
	activeFrom   *time.Time
	activeTo     *time.Time
	maxUses      int
	usedCount    int
	summaryLimit *int64
}

func (p promoAdminContext) toMap() map[string]any {
	return map[string]any{"id": p.id, "code": p.code, "activeFrom": p.activeFrom, "activeTo": p.activeTo, "maxUses": p.maxUses, "usedCount": p.usedCount, "summaryLimit": p.summaryLimit}
}

func (h Handler) loadPromoAdminContext(w http.ResponseWriter, r *http.Request, code string, allowInactive bool) (promoAdminContext, bool) {
	var p promoAdminContext
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "promo is required"})
		return p, false
	}
	if !h.promoAdminTokenValid(r, code) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "promo admin link is invalid"})
		return p, false
	}
	err := h.DB.QueryRow(r.Context(), `select id, code, active_from, active_to, coalesce(max_uses,0), coalesce(used_count,0), summary_limit from promocodes where upper(code)=upper($1)`, code).Scan(&p.id, &p.code, &p.activeFrom, &p.activeTo, &p.maxUses, &p.usedCount, &p.summaryLimit)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "promo not found"})
		return p, false
	}
	if !allowInactive && !promocodeIsActive(p.activeFrom, p.activeTo, p.maxUses, p.usedCount) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "promo is not active"})
		return p, false
	}
	return p, true
}

func promoAdminSummaryWindowActive(activeFrom, activeTo *time.Time) bool {
	now := time.Now()
	if activeFrom != nil && activeFrom.After(now) {
		return false
	}
	if activeTo != nil && activeTo.Before(now) {
		return false
	}
	return true
}

func (h Handler) listSummaryPrompts(ctx context.Context) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, `select id, name, is_default from admin_summary_prompts order by is_default desc, id asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name string
		var def bool
		if err := rows.Scan(&id, &name, &def); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "isDefault": def})
	}
	return out, rows.Err()
}

func (h Handler) promoAdminModes(ctx context.Context, promoID int64) ([]map[string]any, error) {
	var grantsType string
	var targetID int64
	if err := h.DB.QueryRow(ctx, `select coalesce(grants_type, ''), coalesce(target_id, 0) from promocodes where id=$1`, promoID).Scan(&grantsType, &targetID); err != nil {
		return nil, err
	}
	targetIDs, err := h.promocodeTargetIDs(ctx, promoID, targetID)
	if err != nil {
		return nil, err
	}
	modeIDs, err := h.promocodeModeIDsForTargets(ctx, grantsType, targetIDs)
	if err != nil {
		return nil, err
	}
	modeIDs = uniquePositiveIDs(modeIDs)
	if len(modeIDs) == 0 {
		return []map[string]any{}, nil
	}
	rows, err := h.DB.Query(ctx, `
		select id, name
		from modes
		where hidden_at is null
		  and id = any($1::bigint[])
		order by name`, modeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name})
	}
	return out, rows.Err()
}

func (h Handler) promoSummaryUsage(ctx context.Context, promoID int64, limit *int64) (int64, *int64) {
	var used int64
	_ = h.DB.QueryRow(ctx, `select count(*) from admin_export_summaries where promocode_id=$1`, promoID).Scan(&used)
	if limit == nil {
		return used, nil
	}
	remaining := *limit - used
	if remaining < 0 {
		remaining = 0
	}
	return used, &remaining
}

func (h Handler) promoAdminMessageStats(ctx context.Context, promoID int64) (int64, int64, error) {
	promoIDs := []int64{promoID}
	// The JIT compiler spends >1s on a plan of ~90 functions for a ~150ms execution.
	// SET LOCAL jit = off removes the compilation with no speed loss at this data volume.
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL jit = off"); err != nil {
		return 0, 0, err
	}
	// MATERIALIZED forces the planner to materialise promo_user_dialogs before touching
	// dialogs_messages; otherwise it inlines the CTE and picks a SeqScan of the whole table.
	var messageCount, activationsWithoutMessages int64
	err = tx.QueryRow(ctx, `
		with promo_user_dialogs as materialized (
			select ud.id as dialog_id, ud.user_id, pu.used_at as promo_used_at
			from promocode_usages pu
			join users_dialogs ud on ud.user_id = pu.user_id
			where pu.promocode_id = any($2::bigint[])
		),
		promo_msgs as materialized (
			select dm.id, pud.user_id
			from promo_user_dialogs pud
			join dialogs_messages dm on dm.dialog_id = pud.dialog_id
			where dm.role in ('user','assistant')
			  and (
				exists (
					select 1 from dialog_message_access_usage dmau
					join user_mode_access uma on uma.id = dmau.access_id
					where dmau.dialog_message_id = dm.id
					  and uma.access_type = 'promocode'
					  and uma.source_id = any($2::bigint[])
				)
				or (
					not exists (
						select 1 from dialog_message_access_usage dmau_any
						where dmau_any.dialog_message_id = dm.id
					)
					and dm.created_at >= pud.promo_used_at
				)
			  )
		),
		active_users as (select distinct user_id from promo_msgs)
		select
			(select count(*) from promo_msgs),
			(select count(*) from promocode_usages pu
			 where pu.promocode_id = $1
			   and pu.user_id not in (select user_id from active_users))
	`, promoID, promoIDs).Scan(&messageCount, &activationsWithoutMessages)
	return messageCount, activationsWithoutMessages, err
}

func (h Handler) promoAdminSummaryHistory(ctx context.Context, promoID int64, limit int64) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := h.DB.Query(ctx, `
		select id, coalesce(filters, '{}'::jsonb)::text, source_message_count, source_bytes, approx_tokens, result, created_at
		from admin_export_summaries
		where promocode_id=$1
		order by created_at desc, id desc
		limit $2`, promoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type histItem struct {
		id           int64
		filtersJSON  string
		messageCount int64
		sourceBytes  int64
		approxTokens int
		result       string
		createdAt    time.Time
		modeIDs      []int64
	}
	var items []histItem
	modeIDSet := map[int64]struct{}{}

	for rows.Next() {
		var it histItem
		if err := rows.Scan(&it.id, &it.filtersJSON, &it.messageCount, &it.sourceBytes, &it.approxTokens, &it.result, &it.createdAt); err != nil {
			return nil, err
		}
		var filters struct {
			ModeIDs []int64 `json:"modeIds"`
		}
		_ = json.Unmarshal([]byte(it.filtersJSON), &filters)
		it.modeIDs = filters.ModeIDs
		for _, id := range it.modeIDs {
			modeIDSet[id] = struct{}{}
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// One query for all unique modes (instead of N queries).
	modeNames := map[int64]string{}
	if len(modeIDSet) > 0 {
		ids := make([]int64, 0, len(modeIDSet))
		for id := range modeIDSet {
			ids = append(ids, id)
		}
		mrows, err := h.DB.Query(ctx, `select id, name from modes where id = any($1::bigint[])`, ids)
		if err == nil {
			defer mrows.Close()
			for mrows.Next() {
				var mid int64
				var name string
				if err := mrows.Scan(&mid, &name); err == nil {
					modeNames[mid] = name
				}
			}
		}
	}

	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		var label string
		if len(it.modeIDs) == 0 {
			label = "Все режимы промокода"
		} else {
			names := make([]string, 0, len(it.modeIDs))
			for _, id := range it.modeIDs {
				if n, ok := modeNames[id]; ok && strings.TrimSpace(n) != "" {
					names = append(names, n)
				}
			}
			if len(names) == 0 {
				label = "Выбранные режимы"
			} else {
				label = strings.Join(names, ", ")
			}
		}
		out = append(out, map[string]any{
			"ok":           true,
			"summaryId":    it.id,
			"result":       it.result,
			"messageCount": it.messageCount,
			"sourceBytes":  it.sourceBytes,
			"approxTokens": it.approxTokens,
			"createdAt":    it.createdAt,
			"modeLabel":    label,
		})
	}
	return out, nil
}
