package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// The "Department N" practicum task bank: read access for the game and editing for the teacher.
//
// Why not in the service's general admin UI: the teacher has no Mindstrata
// account and should not need one, and the practicum lives on a separate domain. Access uses the same
// system_settings key as the dashboard (see game_results.go).
//
// There is no deletion, only archiving: students' already submitted reports reference the record
// number (answers[].id), as does the "most often failed" summary. A deleted
// row would leave the review without the statement text, and silently.

// gameTaskAnswerOK checks that the answer fits the task. Without this check one could
// put a number into the 1.2 bank; the game would compare it to a boolean truth, never
// match, and the task would become impossible, silently.
func gameTaskAnswerOK(task string, raw json.RawMessage) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	// In practicum 2 quests the answer depends on the task kind and is checked together
	// with its public part; see gameQuestTaskCheck.
	if isQuestTask(task) {
		return false
	}
	switch task {
	case "1.1", "1.4":
		f, ok := v.(float64)
		return ok && f == float64(int(f)) && f >= 1 && f <= 5
	case "1.2":
		_, ok := v.(bool)
		return ok
	case "1.3":
		s, ok := v.(string)
		if !ok {
			return false
		}
		switch s {
		case "fi", "pi", "rnd", "neni":
			return true
		}
		return false
	}
	return false
}

// Tasks of both practicums: 1.x is the discrimination trainer, 2.x are quests over projects
// and preprints.
var gameTaskNames = map[string]bool{
	"1.1": true, "1.2": true, "1.3": true, "1.4": true,
	"2.1": true, "2.2": true,
}

const gameTaskMaxText = 600

// GameTasks handles GET /api/game/tasks?task=1.1, the bank for the game (without a key, since it is
// the task text the student will see anyway), and POST for edits (with a key).
func (h Handler) GameTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.gameTasksList(w, r)
	case http.MethodPost:
		h.gameTasksWrite(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) gameTasksList(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	task := strings.TrimSpace(r.URL.Query().Get("task"))
	if task != "" && !gameTaskNames[task] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестное задание"})
		return
	}
	// Archived ones are returned only with the key: the student has no use for them, while
	// the teacher needs them to restore something removed by accident.
	withArchived := false
	if strings.TrimSpace(r.URL.Query().Get("archived")) == "1" {
		if !h.gameTeacherKeyOK(r.Context(), r) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "неверный ключ преподавателя"})
			return
		}
		withArchived = true
	}
	items, err := h.gameTaskRows(r.Context(), task, withArchived)
	if err != nil {
		h.publicError(r.Context(), w, r, err)
		return
	}
	// The whole bank is not for the student at all. The game takes tasks from the session
	// (game_session.go), the editor and the dashboard use the key. An open list
	// made it possible to collect all 500 statements in advance, and together with the answers (as
	// it was at first), simply to learn them.
	if !h.gameTeacherKeyOK(r.Context(), r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "нужен ключ преподавателя"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": task, "items": items})
}

func (h Handler) gameTaskRows(ctx context.Context, task string, withArchived bool) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, `
		select task, ext_id, text, answer, lvl, src, why, marks, from_doc, archived_at is not null, variant, step, kind, payload
		from game_tasks
		where ($2 = '' or task = $2) and ($3 or archived_at is null)
		  and game = case when $2 like '2.%' then $4 else $1 end
		order by task, variant, step, lvl, ext_id`, gamePraktika1, task, withArchived, gamePraktika2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var t, text, src, why string
		var extID, lvl int
		var answer json.RawMessage
		var marks []int32
		var fromDoc, archived bool
		var variant, step int
		var kind string
		var payload json.RawMessage
		if err := rows.Scan(&t, &extID, &text, &answer, &lvl, &src, &why, &marks, &fromDoc, &archived, &variant, &step, &kind, &payload); err != nil {
			return nil, err
		}
		m := make([]int, 0, len(marks))
		for _, v := range marks {
			m = append(m, int(v))
		}
		out = append(out, map[string]any{
			"task": t, "id": extID, "t": text, "answer": answer, "lvl": lvl,
			"src": src, "why": why, "marks": m, "doc": fromDoc, "archived": archived,
			"variant": variant, "step": step, "kind": kind, "payload": payload,
		})
	}
	return out, rows.Err()
}

// gameTaskIn is the edit body. Parsing is strict, so the field set matches
// what the editor sends.
type gameTaskIn struct {
	Task    string          `json:"task"`
	ID      int             `json:"id"`
	Variant int             `json:"variant"`
	Step    int             `json:"step"`
	Text    string          `json:"t"`
	Answer  json.RawMessage `json:"answer"`
	Lvl     int             `json:"lvl"`
	Src     string          `json:"src"`
	Why     string          `json:"why"`
	Marks   []int           `json:"marks"`
	FromDoc bool            `json:"doc"`
	// Practicum 2 quests only: the task kind (rate | choice) and what
	// the student sees: the fragment text, the scale, the options.
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func (h Handler) gameTasksWrite(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	if !h.gameTeacherKeyOK(r.Context(), r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "неверный ключ преподавателя"})
		return
	}
	var req struct {
		Action string       `json:"action"` // save | archive | restore | import
		Item   gameTaskIn   `json:"item"`
		Items  []gameTaskIn `json:"items"`
	}
	// 4 MB: the practicum 2 bank is 120 application fragments with reviews; in
	// Cyrillic it weighs about a megabyte and did not fit into the old limit.
	if err := decodeJSONStrict(w, r, 4<<20, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	ctx := r.Context()

	switch strings.TrimSpace(req.Action) {
	case "save":
		if code, err := h.gameTaskSave(ctx, req.Item); err != nil {
			writeJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "archive", "restore":
		if !gameTaskNames[req.Item.Task] {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестное задание"})
			return
		}
		set := "archived_at = now()"
		if req.Action == "restore" {
			set = "archived_at = null"
		}
		// The bank is chosen by task: this used to always be the practicum 1 bank,
		// and 2.x tasks could be neither archived nor restored.
		tag, err := h.DB.Exec(ctx, `update game_tasks set `+set+`, updated_at = now()
			where game = $1 and task = $2 and ext_id = $3`, gameForTask(req.Item.Task), req.Item.Task, req.Item.ID)
		if err != nil {
			h.publicError(ctx, w, r, err)
			return
		}
		if tag.RowsAffected() == 0 {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "запись не найдена"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "import":
		// Loading the bank from files. It comes from the client because the JSON lives next to
		// the static files in the Caddy container, not with the API. Existing rows
		// are updated and other people's edits are not overwritten: the import can be repeated without surprises.
		//
		// Rejections are returned as a list: bad records used to be skipped silently,
		// and the only way to find out why "118 of 120 loaded" was trial and error.
		saved := 0
		rejected := []map[string]any{}
		for _, it := range req.Items {
			if _, err := h.gameTaskSave(ctx, it); err != nil {
				if len(rejected) < 20 {
					rejected = append(rejected, map[string]any{"task": it.Task, "id": it.ID, "error": err.Error()})
				}
				continue
			}
			saved++
		}
		h.writeAdminAudit(ctx, r, 0, "admin.game_task.import", "game_tasks", nil,
			map[string]any{"received": len(req.Items), "saved": saved})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "saved": saved, "received": len(req.Items), "rejected": rejected})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестное действие"})
	}
}

func (h Handler) gameTaskSave(ctx context.Context, it gameTaskIn) (int, error) {
	if !gameTaskNames[it.Task] {
		return http.StatusBadRequest, errGameTask("неизвестное задание")
	}
	text := strings.Join(strings.Fields(it.Text), " ")
	if len([]rune(text)) < 3 || len([]rune(text)) > gameTaskMaxText {
		return http.StatusBadRequest, errGameTask("текст утверждения от 3 до 600 знаков")
	}
	kind := gameKindBinary
	payload := json.RawMessage(`{}`)
	if isQuestTask(it.Task) {
		kind = strings.TrimSpace(it.Kind)
		if err := gameQuestTaskCheck(kind, it.Payload, it.Answer); err != nil {
			return http.StatusBadRequest, errGameTask(err.Error())
		}
		payload = it.Payload
	} else if !gameTaskAnswerOK(it.Task, it.Answer) {
		return http.StatusBadRequest, errGameTask("ответ не подходит этому заданию")
	}
	if it.Lvl < 1 || it.Lvl > 3 {
		return http.StatusBadRequest, errGameTask("уровень от 1 до 3")
	}
	if it.ID <= 0 {
		return http.StatusBadRequest, errGameTask("нужен номер записи")
	}
	marks := it.Marks
	if marks == nil {
		marks = []int{}
	}
	_, err := h.DB.Exec(ctx, `
		insert into game_tasks (game, task, ext_id, text, answer, lvl, src, why, marks, from_doc, variant, step, kind, payload)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		on conflict (game, task, ext_id) do update
		set text = excluded.text, answer = excluded.answer, lvl = excluded.lvl,
		    src = excluded.src, why = excluded.why, marks = excluded.marks,
		    from_doc = excluded.from_doc, variant = excluded.variant, step = excluded.step,
		    kind = excluded.kind, payload = excluded.payload,
		    archived_at = null, updated_at = now()`,
		gameForTask(it.Task), it.Task, it.ID, text, it.Answer, it.Lvl,
		strings.TrimSpace(it.Src), strings.TrimSpace(it.Why), marks, it.FromDoc, it.Variant, it.Step, kind, payload)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

type errGameTask string

func (e errGameTask) Error() string { return string(e) }

// gameTeacherKeyOK is the shared key check for the dashboard and the editor. The key is taken
// from the query (?key=) or from a header, so it does not end up in proxy logs when
// that matters.
func (h Handler) gameTeacherKeyOK(ctx context.Context, r *http.Request) bool {
	// Guessing is already under way: nobody from this address gets in, even with the correct
	// key; otherwise the ceiling is bypassed by guessing on the last attempt.
	if locked, _ := h.keyAttemptsExhausted(r); locked {
		return false
	}
	want, err := h.systemSetting(ctx, gameTeacherKeySetting)
	if err != nil || strings.TrimSpace(want) == "" {
		return false
	}
	// A header is preferred over the query string: a key in the URL ends up in proxy
	// logs and browser history and leaks via Referer to third-party resources.
	got := strings.TrimSpace(r.Header.Get("X-Teacher-Key"))
	if got == "" {
		got = strings.TrimSpace(r.URL.Query().Get("key"))
	}
	if got == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
		return true
	}
	h.guardKeyAttempt(ctx, r, "teacher")
	return false
}

// gameTaskExtIDFromPath extracts the record number from a path like /api/game/tasks/123.
// Kept as a separate function so the route and the parsing do not drift apart.
func gameTaskExtIDFromPath(p string) (int, bool) {
	rest := strings.TrimPrefix(p, "/api/game/tasks/")
	if rest == p || rest == "" {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil
}
