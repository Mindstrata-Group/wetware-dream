package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The "Department N" practicum game session is the only honest way to play.
//
// Background: the task bank was sent to the client together with the answers, and the points
// arrived from the browser as a ready number. The IRIT-RTF students, who had just been
// given a hacking assignment, made exactly the two obvious moves: they downloaded the answers and
// submitted a score of 9,999,999. Neither can be fixed by client-side checks:
// the client holds everything it wants to tamper with.
//
// So now:
//   - the server hands out tasks WITHOUT answers (start);
//   - the server checks each answer and only it knows the truth (answer);
//   - the server computes the points from time it measures itself (finish).
//
// The client still shows the verdict instantly; it just gets it from the
// server's response instead of computing it.

const (
	gameSessionRounds = 12            // this many tasks in one session
	gameSessionTTL    = 2 * time.Hour // nobody plays longer than the practicum
	// The practicum 2 quest is not a five-minute shooting range: the four-week "Glavred"
	// easily stretches over a class and an evening at home. Two hours would cut it off
	// midway with "session expired".
	gameQuestSessionTTL = 12 * time.Hour
	gameAnswerGraceMS   = 8000 // headroom over the time limit for the network and thinking
	gameMinPlausibleMS  = 250  // a human cannot read a statement faster than this
	gameMaxSessionsHour = 40   // sessions per full name per hour
)

// gameTaskTime is the per-level time limit in milliseconds. It must match
// TASKS[...].time in game.js: the client draws the bar, the server counts lateness.
var gameTaskTime = map[string]map[int]int{
	"1.1": {1: 16000, 2: 21000, 3: 28000},
	"1.2": {1: 13000, 2: 17000, 3: 23000},
	"1.3": {1: 52000, 2: 68000, 3: 92000},
	"1.4": {1: 40000, 2: 55000, 3: 75000},
	// Practicum 2 quests have no stopwatch: students read the project description and
	// think rather than race. The limit only exists so that an abandoned
	// tab does not keep a session forever.
	"2.1": {1: 600000, 2: 600000, 3: 600000},
	"2.2": {1: 600000, 2: 600000, 3: 600000},
}

var gameBase = map[int]int{1: 100, 2: 150, 3: 220}
var gameDamage = map[int]int{1: 25, 2: 30, 3: 35}

// GameSession handles POST /api/game/session with the action field: start | show | answer |
// finish | restore | log.
//
// One route for several actions is deliberate: every new path has to be added to
// two route registries and to OpenAPI, and the price of forgetting is a red CI
// for nothing.
func (h Handler) GameSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	if !h.guardGameCall(r.Context(), w, r) {
		return
	}
	var req struct {
		Action    string            `json:"action"`
		SessionID string            `json:"sessionId"`
		Task      string            `json:"task"`
		Family    string            `json:"family"`
		Name      string            `json:"name"`
		Group     string            `json:"group"`
		Mate      string            `json:"mate"`
		Variant   int               `json:"variant"`
		ItemID    int               `json:"itemId"`
		Value     any               `json:"value"`
		TimedOut  bool              `json:"timedOut"`
		Events    []json.RawMessage `json:"events"`
	}
	// 64 KB: a batch of log events (up to 50) is larger than a regular answer.
	if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	ctx := r.Context()

	switch strings.TrimSpace(req.Action) {
	case "start":
		h.gameSessionStart(ctx, w, r, req.Task, req.Family, req.Name, req.Group, req.Mate, req.Variant)
	case "show":
		h.gameSessionShow(ctx, w, r, req.SessionID, req.ItemID)
	case "answer":
		h.gameSessionAnswer(ctx, w, r, req.SessionID, req.ItemID, req.Value, req.TimedOut)
	case "finish":
		h.gameSessionFinish(ctx, w, r, req.SessionID)
	case "restore":
		h.gameSessionRestore(ctx, w, r, req.Family, req.Name, req.Group)
	case "log":
		h.gameSessionLog(ctx, w, r, req.SessionID, req.Events)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестное действие"})
	}
}

func (h Handler) gameSessionStart(ctx context.Context, w http.ResponseWriter, r *http.Request, task, family, name, group, mate string, variant int) {
	if !gameTaskNames[task] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестное задание"})
		return
	}
	family, name, group = clipName(family), clipName(name), clipName(group)
	if family == "" || name == "" || group == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужны фамилия, имя и группа"})
		return
	}

	// A cap on the number of sessions: without it one could spin start in a loop until
	// a convenient set of tasks comes up, and litter the database along the way.
	var recent int
	if err := h.DB.QueryRow(ctx, `
		select count(*) from game_sessions
		where lower(trim(family)) = lower(trim($1)) and lower(trim(name)) = lower(trim($2))
		  and lower(trim(student_group)) = lower(trim($3)) and started_at > now() - interval '1 hour'`,
		family, name, group).Scan(&recent); err == nil && recent >= gameMaxSessionsHour {
		h.logSecurityEvent(ctx, "rate_limited", "warn", r, map[string]any{"family": family, "task": task, "sessions": recent})
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "слишком много попыток подряд, подождите"})
		return
	}

	// The task set. In practicum 1 it is a random sample by level: there students train
	// discrimination on a stream of examples. In practicum 2 it is a fixed set of steps
	// of one variant: the teacher names the project number, and the review goes
	// through exactly that one, stage by stage.
	var rows pgx.Rows
	var err error
	if isQuestTask(task) {
		if variant <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужен номер варианта — его называет преподаватель"})
			return
		}
		rows, err = h.DB.Query(ctx, `
			select ext_id, text, lvl, src, kind, payload from game_tasks
			where game=$1 and task=$2 and variant=$3 and archived_at is null
			order by step, ext_id`, gamePraktika2, task, variant)
	} else {
		rows, err = h.DB.Query(ctx, `
			(select ext_id, text, lvl, src, kind, payload from game_tasks where game=$1 and task=$2 and archived_at is null and lvl=1 order by random() limit 4)
			union all
			(select ext_id, text, lvl, src, kind, payload from game_tasks where game=$1 and task=$2 and archived_at is null and lvl=2 order by random() limit 4)
			union all
			(select ext_id, text, lvl, src, kind, payload from game_tasks where game=$1 and task=$2 and archived_at is null and lvl=3 order by random() limit 4)`,
			gamePraktika1, task)
	}
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID      int            `json:"id"`
		T       string         `json:"t"`
		Lvl     int            `json:"lvl"`
		Src     string         `json:"src"`
		Kind    string         `json:"kind,omitempty"`
		Payload map[string]any `json:"payload,omitempty"`
	}
	items := []item{}
	ids := []int{}
	for rows.Next() {
		var it item
		var kind string
		var payload json.RawMessage
		if err := rows.Scan(&it.ID, &it.T, &it.Lvl, &it.Src, &kind, &payload); err != nil {
			h.publicError(ctx, w, r, err)
			return
		}
		if kind != gameKindBinary {
			it.Kind = kind
			it.Payload = gameQuestPublic(kind, payload)
		}
		items = append(items, it)
		ids = append(ids, it.ID)
	}
	if err := rows.Err(); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	minItems := gameSessionRounds
	if isQuestTask(task) {
		minItems = 2 // a quest has few steps, and all of them are mandatory
	}
	if len(items) < minItems {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok": false, "error": "для этого варианта задания ещё не заведены",
		})
		return
	}

	game := gamePraktika1
	if isQuestTask(task) {
		game = gamePraktika2
	}
	var sessionID string
	if err := h.DB.QueryRow(ctx, `
		insert into game_sessions (game, task, family, name, student_group, item_ids, ip_hash, variant, mate)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9) returning id::text`,
		game, task, family, name, group, ids, requestIPHash(r), variant, clipName(mate)).Scan(&sessionID); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "sessionId": sessionID, "items": items, "rounds": len(items),
	})
}

// gameSessionShow: a quest task has appeared on screen.
//
// In quests the student plays between tasks: a week in "Glavred" easily
// takes a quarter of an hour. Answer time used to be measured from the previous answer, and
// an honest answer after a long scene counted as "late". Now the page
// reports when the task was shown, and timing starts from that moment.
//
// Only the task the server already expects next can be marked, and
// only in a quest: in the practicum 1 shooting range, time is part of the game.
func (h Handler) gameSessionShow(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string, itemID int) {
	if !uuidPattern.MatchString(strings.TrimSpace(sessionID)) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужен sessionId"})
		return
	}
	var task string
	var ids []int32
	var answersRaw []byte
	var finished *time.Time
	err := h.DB.QueryRow(ctx, `
		select task, item_ids, answers, finished_at from game_sessions where id = $1::uuid`, sessionID).
		Scan(&task, &ids, &answersRaw, &finished)
	if errors.Is(err, pgx.ErrNoRows) {
		h.logSecurityEvent(ctx, "alien_session", "warn", r, map[string]any{"sessionId": sessionID})
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "сессия не найдена"})
		return
	}
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	if !isQuestTask(task) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "только для квестов"})
		return
	}
	if finished != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "сессия уже закрыта"})
		return
	}
	var given []map[string]any
	_ = json.Unmarshal(answersRaw, &given)
	if len(given) >= len(ids) || int(ids[len(given)]) != itemID {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "это задание сейчас не ждут"})
		return
	}
	if _, err := h.DB.Exec(ctx, `update game_sessions set last_seen_at = now() where id = $1::uuid`, sessionID); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Game log: how many events we accept at once, how long each may be, and
// how many of the latest we keep in the session.
const (
	gameLogBatchMax = 50
	gameLogEventMax = 800
	gameLogKeepLast = 1000
)

// gameSessionLog appends page events to the session log: quest steps,
// exams, browser errors. Without it, a "everything crashed" complaint was investigated by
// guesswork: the server knew the answers but not what the student did in the quest.
//
// The log does not affect points and is not checked for truthfulness: these are the browser's
// testimony, not facts. That is why it is accepted even from a closed session:
// the final events arrive right after closing.
func (h Handler) gameSessionLog(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string, events []json.RawMessage) {
	if !uuidPattern.MatchString(strings.TrimSpace(sessionID)) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужен sessionId"})
		return
	}
	if len(events) == 0 || len(events) > gameLogBatchMax {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "от 1 до 50 событий за раз"})
		return
	}
	for _, ev := range events {
		var obj map[string]any
		if len(ev) > gameLogEventMax || json.Unmarshal(ev, &obj) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "событие — объект до 800 байт"})
			return
		}
	}
	batch, _ := json.Marshal(events)
	var kept int
	err := h.DB.QueryRow(ctx, `
		update game_sessions set client_log = (
			select coalesce(jsonb_agg(e order by o), '[]'::jsonb) from (
				select e, o from jsonb_array_elements(client_log || $2::jsonb) with ordinality as x(e, o)
				order by o desc limit $3) t)
		where id = $1::uuid
		returning jsonb_array_length(client_log)`, sessionID, batch, gameLogKeepLast).Scan(&kept)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "сессия не найдена"})
		return
	}
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "kept": kept})
}

// gameSessionAnswer is the only place where correctness is checked.
func (h Handler) gameSessionAnswer(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string, itemID int, value any, timedOut bool) {
	if strings.TrimSpace(sessionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужен sessionId"})
		return
	}
	var task string
	var ids []int32
	var answersRaw []byte
	var armor int
	var finished *time.Time
	var startedAt, lastSeen time.Time
	err := h.DB.QueryRow(ctx, `
		select task, item_ids, answers, armor, finished_at, started_at, last_seen_at
		from game_sessions where id = $1::uuid`, sessionID).
		Scan(&task, &ids, &answersRaw, &armor, &finished, &startedAt, &lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		h.logSecurityEvent(ctx, "alien_session", "warn", r, map[string]any{"sessionId": sessionID})
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "сессия не найдена"})
		return
	}
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	if finished != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "сессия уже закрыта"})
		return
	}
	if time.Since(startedAt) > gameTTLFor(task) {
		writeJSON(w, http.StatusGone, map[string]any{"ok": false, "error": "сессия устарела, начните заново"})
		return
	}

	var given []map[string]any
	_ = json.Unmarshal(answersRaw, &given)
	if len(given) >= len(ids) {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "на все задания уже отвечено"})
		return
	}
	// Repeating an answer to an already answered task is not a violation but a lost
	// server response.
	//
	// On a connection drop the client offers to answer again, but the request may have arrived
	// while the response was lost. Then the repeat arrived for a task the server
	// had already closed and was rejected with "answer to the wrong task". On 23.09 one student
	// got into this loop four times in a row: the answer was counted, but the screen showed
	// an error, and the person restarted the task. This explains part of the complaints about "too
	// many attempts".
	//
	// So we return the previous verdict: it cannot be rewritten, so an answer cannot be
	// brute-forced by repeating either.
	for _, a := range given {
		if n, ok := a["id"].(float64); ok && int(n) == itemID {
			h.replayAnswer(ctx, w, r, sessionID, task, itemID, a, len(given), len(ids))
			return
		}
	}

	// Only the current task can be answered, strictly in order: otherwise
	// one could "re-answer" a miss or send answers in a batch.
	expected := int(ids[len(given)])
	if itemID != expected {
		// This is a client desync, not someone else's session: the session is the right one, just
		// the submitted task is not. A separate kind so that real attempts to
		// break into someone else's game are not lost among these.
		h.logSecurityEvent(ctx, "out_of_order", "warn", r, map[string]any{
			"sessionId": sessionID, "sent": itemID, "expected": expected,
		})
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "ответ не на то задание"})
		return
	}

	var truthRaw, payloadRaw json.RawMessage
	var why, kind string
	var lvl int
	if err := h.DB.QueryRow(ctx, `
		select answer, why, lvl, kind, payload from game_tasks
		where game=$1 and task=$2 and ext_id=$3`, gameForTask(task), task, itemID).Scan(&truthRaw, &why, &lvl, &kind, &payloadRaw); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}

	// The server measures time: from the previous answer (or the start) to this one.
	spentMS := int(time.Since(lastSeen).Milliseconds())
	limit := gameTaskTime[task][lvl]
	if limit == 0 {
		limit = 20000
	}
	late := spentMS > limit+gameAnswerGraceMS
	if late {
		timedOut = true
	}
	// An answer faster than a human can give is not counted.
	//
	// Such an answer used to be only logged, while the points for it counted as for
	// an honest one. On 23.09 this was exploited: a session stayed open for 17 minutes, and
	// then 11 answers went out in 1.4 seconds at 125-141 ms each, eight correct, all
	// with different options. That is replaying a list prepared in advance.
	//
	// There is no and never was a hole in the protection: correct answers never leave the server, but
	// it must show the TEXTS of the statements, otherwise the game cannot be played. They
	// are collected over several sessions, worked out in advance and fed in by a script.
	// A multiple-choice test cannot be protected any other way; what remains is time, and it
	// reliably tells reading from substitution: 125 ms is not enough to read
	// even the heading.
	//
	// So the threshold turns from an observation into a rule: such an answer counts as
	// wrong and is logged as an alarm rather than a note.
	tooFast := !timedOut && spentMS < gameMinPlausibleMS
	if tooFast {
		h.logSecurityEvent(ctx, "impossible_timing", "alert", r, map[string]any{
			"sessionId": sessionID, "itemId": itemID, "ms": spentMS, "counted": false,
		})
	}

	// credit is the share of the point for an answer. In practicum 1 tasks it is 0 or 1; in
	// practicum 2 quests it can be partial (game_quest_scoring.go).
	credit := 0.0
	if !timedOut && !tooFast {
		if kind == gameKindRate || kind == gameKindChoice {
			credit = gameCreditRound(gameQuestCredit(kind, truthRaw, value))
		} else {
			var truth any
			_ = json.Unmarshal(truthRaw, &truth)
			if gameAnswersEqual(truth, value) {
				credit = 1
			}
		}
	}
	correct := credit >= 0.999

	delta := 0
	switch {
	case correct:
		speed := 1 - float64(spentMS)/float64(limit)
		if speed < 0 {
			speed = 0
		}
		delta = int(math.Round(float64(gameBase[lvl]) * (1 + speed*0.5)))
	case credit > 0:
		// A defensible but incomplete answer: points in proportion to the share, no penalty.
		delta = int(math.Round(float64(gameBase[lvl]) * credit))
	default:
		delta = -gameBase[lvl]
		armor -= gameDamage[lvl]
	}

	// "given" is what exactly the student chose. It used to be not stored, and in the
	// report assembled by the server the "your answer" column stayed empty: one could see
	// "wrong" but not what was wrong. For the review at the defence this is exactly the
	// most needed field.
	rec := map[string]any{
		"id": itemID, "lvl": lvl, "correct": correct, "timedOut": timedOut, "ms": spentMS,
		"given": value, "credit": credit,
	}
	if gameQuestBonus(payloadRaw) {
		rec["bonus"] = true
	}
	given = append(given, rec)
	newAnswers, _ := json.Marshal(given)
	var score, correctCount int
	if err := h.DB.QueryRow(ctx, `
		update game_sessions
		set answers = $2::jsonb,
		    score = greatest(0, score + $3),
		    correct = correct + case when $4 then 1 else 0 end,
		    armor = $5,
		    last_seen_at = now()
		where id = $1::uuid
		returning score, correct`, sessionID, newAnswers, delta, correct, armor).Scan(&score, &correctCount); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}

	var truth any
	_ = json.Unmarshal(truthRaw, &truth)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "correct": correct, "credit": credit, "timedOut": timedOut, "truth": truth, "why": why,
		"delta": delta, "score": score, "armor": armor, "ms": spentMS,
		"answered": len(given), "rounds": len(ids), "tooFast": tooFast,
	})
}

func (h Handler) gameSessionFinish(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужен sessionId"})
		return
	}
	var task, family, name, group string
	var answersRaw []byte
	var score, correctCount, armor int
	var ids []int32
	var finished *time.Time
	var startedAt time.Time
	err := h.DB.QueryRow(ctx, `
		select task, family, name, student_group, answers, score, correct, armor, item_ids, finished_at, started_at
		from game_sessions where id = $1::uuid`, sessionID).
		Scan(&task, &family, &name, &group, &answersRaw, &score, &correctCount, &armor, &ids, &finished, &startedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "сессия не найдена"})
		return
	}
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	if finished == nil {
		if _, err := h.DB.Exec(ctx, `update game_sessions set finished_at = now() where id = $1::uuid`, sessionID); err != nil {
			h.publicError(ctx, w, r, err)
			return
		}
	}

	var given []map[string]any
	_ = json.Unmarshal(answersRaw, &given)
	h.flagUniformPace(ctx, r, sessionID, family, group, given)
	main, bonus := gameCreditSums(given)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "task": task, "score": score, "correct": correctCount,
		"total": len(given), "rounds": len(ids), "armor": armor,
		"credit": main.sum, "creditTotal": main.n, "bonusCredit": bonus.sum, "bonusTotal": bonus.n,
		"durationMs": time.Since(startedAt).Milliseconds(), "answers": given,
	})
}

// flagUniformPace catches play from a list prepared in advance, including one
// worked out by a neural network.
//
// The 250 ms threshold is trivially bypassed: just add a pause to the script.
// But an EVEN pace is harder to fake, and there is no reason to: the script produces it by itself.
// Real students on 23.09 had answer times ranging from 3.5 to 25 seconds, i.e.
// a spread comparable to the mean. On the same day the script's eleven
// answers all fit into 125-141 ms: a spread of four percent of the mean.
//
// We measure the coefficient of variation: standard deviation divided by the mean.
// A human does not stay below 15% even when rushing, because they read
// statements of different length and difficulty.
//
// Logging only, with no effect on points: the sign is strong but
// indirect, and the decision based on it is the teacher's.
func (h Handler) flagUniformPace(ctx context.Context, r *http.Request, sessionID, family, group string, given []map[string]any) {
	const minAnswers = 8 // with fewer answers the spread itself is unstable
	times := make([]float64, 0, len(given))
	for _, a := range given {
		if v, ok := a["ms"].(float64); ok && v > 0 {
			times = append(times, v)
		}
	}
	if len(times) < minAnswers {
		return
	}
	mean := 0.0
	for _, t := range times {
		mean += t
	}
	mean /= float64(len(times))
	if mean <= 0 {
		return
	}
	variance := 0.0
	for _, t := range times {
		variance += (t - mean) * (t - mean)
	}
	cv := math.Sqrt(variance/float64(len(times))) / mean
	if cv >= 0.15 {
		return
	}
	h.logSecurityEvent(ctx, "uniform_pace", "alert", r, map[string]any{
		"sessionId": sessionID, "family": family, "group": group,
		"answers": len(times), "meanMs": int(mean), "cv": math.Round(cv*1000) / 1000,
	})
}

// gameAnswersEqual compares the student's answer with the correct one. JSON brings numbers
// as float64, so 3 and 3.0 must count as the same; otherwise
// correct answers in 1.1 and 1.4 would silently count as misses.
func gameAnswersEqual(truth, given any) bool {
	switch t := truth.(type) {
	case bool:
		g, ok := given.(bool)
		return ok && g == t
	case string:
		g, ok := given.(string)
		return ok && g == t
	case float64:
		switch g := given.(type) {
		case float64:
			return math.Abs(g-t) < 1e-9
		case string:
			var parsed float64
			if _, err := fmt.Sscanf(g, "%g", &parsed); err == nil {
				return math.Abs(parsed-t) < 1e-9
			}
		}
	}
	return false
}

// logSecurityEvent records a trace of someone else's attempt. The write error is swallowed on purpose:
// the security log must not break the game of a student who has nothing to do with it.
func (h Handler) logSecurityEvent(ctx context.Context, kind, severity string, r *http.Request, detail map[string]any) {
	if h.DB == nil {
		return
	}
	path := ""
	if r != nil {
		path = r.URL.Path
	}
	meta, _ := json.Marshal(detail)
	_, _ = h.DB.Exec(ctx, `
		insert into security_events (kind, severity, ip_hash, path, detail)
		values ($1,$2,$3,$4,$5::jsonb)`, kind, severity, requestIPHash(r), path, meta)
}

// isQuestTask: practicum 2 tasks are played as a quest: a fixed set of steps per
// variant, with no stopwatch and no random sampling.
func isQuestTask(task string) bool { return strings.HasPrefix(task, "2.") }

// gameTTLFor is how long a task session lives.
func gameTTLFor(task string) time.Duration {
	if isQuestTask(task) {
		return gameQuestSessionTTL
	}
	return gameSessionTTL
}

func gameForTask(task string) string {
	if isQuestTask(task) {
		return gamePraktika2
	}
	return gamePraktika1
}

// gameSessionRestore returns the best attempt of each task.
//
// Why: the browser kept progress only in page memory, and a reload
// wiped everything; on 23.09 students replayed completed tasks in circles because of this and
// burned through the session cap. Meanwhile the server knew the result all along: it
// checks the answers and computes the points itself.
//
// We return the best attempt, not the last one: the teacher promised to count
// the best result, and restore must behave the same way; otherwise
// an unlucky restart would overwrite a good game.
//
// A check for someone else's data is no more needed here than in the report: session identifiers
// are issued by full name and group, and the same set is checked again on submission
// (gameScoreFromSessions). One cannot claim someone else's points this way.
func (h Handler) gameSessionRestore(ctx context.Context, w http.ResponseWriter, r *http.Request, family, name, group string) {
	family, name, group = clipName(family), clipName(name), clipName(group)
	if family == "" || name == "" || group == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужны фамилия, имя и группа"})
		return
	}

	// Auto-closing of abandoned attempts.
	//
	// The score, armour and answers are written on EVERY answer, so an interrupted game
	// is already fully in the database; it only lacks the closing mark. Without
	// it, nobody counts the result: neither restore nor the report. This is exactly
	// how a 27-point game was lost for a student who closed the tab on 23.09.
	//
	// We close it retroactively with the time of the last answer, not now(): otherwise
	// a session abandoned at 10:00 would be credited with a duration until the moment the student
	// happened to come back to the page in the evening.
	//
	// A minute of silence, so we do not close a game that is being played right now in
	// a neighbouring tab.
	if _, err := h.DB.Exec(ctx, `
		update game_sessions set finished_at = last_seen_at
		where finished_at is null
		  and last_seen_at < now() - interval '1 minute'
		  and jsonb_array_length(answers) > 0
		  -- Квест между экзаменами идёт по четверти часа без единого ответа.
		  -- Закрыть его «по минуте тишины» значит оборвать живую игру при
		  -- первом же обновлении страницы: дальше каждый ответ получал бы
		  -- «сессия уже закрыта». Квест закрываем, только когда он истёк.
		  and (task not like '2.%' or started_at < now() - interval '12 hours')
		  and lower(trim(family)) = lower(trim($1))
		  and lower(trim(name)) = lower(trim($2))
		  and lower(trim(student_group)) = lower(trim($3))`,
		family, name, group); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}

	// distinct on + order by: postgres returns one row per task, the
	// first in its group, i.e. the one with the highest score.
	rows, err := h.DB.Query(ctx, `
		select distinct on (task)
		       task, id::text, score, correct, armor, variant, mate,
		       jsonb_array_length(answers) as answered,
		       extract(epoch from (finished_at - started_at)) * 1000 as duration_ms
		from game_sessions
		where lower(trim(family)) = lower(trim($1))
		  and lower(trim(name)) = lower(trim($2))
		  and lower(trim(student_group)) = lower(trim($3))
		  and finished_at is not null
		order by task, score desc, finished_at desc`,
		family, name, group)
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	ids := []string{}
	for rows.Next() {
		var task, id, mate string
		var score, correct, armor, variant, answered int
		var durationMS float64
		if err := rows.Scan(&task, &id, &score, &correct, &armor, &variant, &mate, &answered, &durationMS); err != nil {
			h.publicError(ctx, w, r, err)
			return
		}
		out = append(out, map[string]any{
			"task": task, "sessionId": id, "score": score, "correct": correct,
			"total": answered, "armor": armor, "variant": variant, "mate": mate,
			"durationMs": int64(durationMS), "answers": []map[string]any{},
		})
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	if len(ids) > 0 {
		if err := h.attachRestoredAnswers(ctx, out, ids); err != nil {
			h.publicError(ctx, w, r, err)
			return
		}
	}
	// Partial credit in quests: without it the restored case would show
	// "3 of 12 correct" instead of "8.4 points of 12".
	for _, o := range out {
		given, _ := o["answers"].([]map[string]any)
		m, b := gameCreditSums(given)
		o["credit"], o["creditTotal"], o["bonusCredit"], o["bonusTotal"] = m.sum, m.n, b.sum, b.n
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "best": out})
}

// attachRestoredAnswers adds the full review to restored tasks.
//
// Without it, a student who came back after a page reload would download a report with
// an empty table: the session holds task numbers, while the form from the course manual requires
// the statement text, the student's answer, the correct answer and the justification. Such a
// report cannot be submitted, so we rebuild the rows using texts from the bank.
//
// Correct answers can and should be returned here: the tasks are already played and closed,
// and the student saw the review right after each answer.
func (h Handler) attachRestoredAnswers(ctx context.Context, out []map[string]any, ids []string) error {
	// In one query: unfold the session's answers into rows and immediately match
	// each with the task text from the bank. ordinality preserves the answer order.
	rows, err := h.DB.Query(ctx, `
		select s.id::text, a.item,
		       coalesce(t.text,''), coalesce(t.why,''), coalesce(t.answer,'null'::jsonb),
		       coalesce(t.src,''), coalesce(t.marks, '{}')
		from game_sessions s
		     cross join lateral jsonb_array_elements(s.answers) with ordinality as a(item, ord)
		     left join game_tasks t
		            on t.game = s.game and t.task = s.task
		           and t.ext_id = (a.item->>'id')::int
		where s.id = any($1::uuid[])
		order by s.id, a.ord`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	bySession := map[string][]map[string]any{}
	for rows.Next() {
		var sid, text, why, src string
		var itemRaw, answerRaw []byte
		var marks []int32
		if err := rows.Scan(&sid, &itemRaw, &text, &why, &answerRaw, &src, &marks); err != nil {
			return err
		}
		row := map[string]any{}
		_ = json.Unmarshal(itemRaw, &row) // id, lvl, correct, timedOut, ms, given
		var truth any
		_ = json.Unmarshal(answerRaw, &truth)
		if marks == nil {
			marks = []int32{}
		}
		row["t"], row["why"], row["truth"], row["src"], row["marks"] = text, why, truth, src, marks
		bySession[sid] = append(bySession[sid], row)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range out {
		sid, _ := item["sessionId"].(string)
		if got := bySession[sid]; got != nil {
			item["answers"] = got
		}
	}
	return nil
}

// replayAnswer repeats the previous verdict for an already answered task.
//
// It is needed where the server's response was lost on the way: the student sees "connection
// lost" and answers again, while the server has already recorded that answer. The repeat used to be
// rejected, and the person restarted the whole task.
//
// Nothing changes: no points, no armour, no session record. So an answer cannot be
// brute-forced by repeating: the verdict is always the same as the first time.
func (h Handler) replayAnswer(ctx context.Context, w http.ResponseWriter, r *http.Request,
	sessionID, task string, itemID int, prev map[string]any, answered, rounds int) {

	var truthRaw json.RawMessage
	var why string
	if err := h.DB.QueryRow(ctx, `
		select answer, why from game_tasks
		where game=$1 and task=$2 and ext_id=$3`, gameForTask(task), task, itemID).Scan(&truthRaw, &why); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	var truth any
	_ = json.Unmarshal(truthRaw, &truth)

	var score, armor int
	if err := h.DB.QueryRow(ctx,
		`select score, armor from game_sessions where id = $1::uuid`, sessionID).Scan(&score, &armor); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}

	num := func(key string) int {
		if v, ok := prev[key].(float64); ok {
			return int(v)
		}
		return 0
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "correct": prev["correct"] == true, "timedOut": prev["timedOut"] == true,
		"truth": truth, "why": why, "delta": 0, "score": score, "armor": armor,
		"ms": num("ms"), "answered": answered, "rounds": rounds,
		"tooFast": false, "replayed": true,
	})
}
