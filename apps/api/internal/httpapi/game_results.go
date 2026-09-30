package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The "Department N" training game (scientific shooting range), an IRIT-RTF practicum.
//
// Two public routes without authorisation: the student submits a result and
// views the cohort statistics. There is deliberately no authorisation: creating an account
// for one seminar game means losing half the group on the sign-up
// form. The cost of this decision is stated honestly: one can sign with someone else's name;
// this is a trainer, not an exam.
//
// For the same reason there is nothing worth stealing here: no personal
// data beyond the full name and group that the student writes in their notebook anyway, and no link to
// service accounts.

// gamePraktika1 is the whole practical session 1: four tasks, four
// retro mechanics, one report at the end. gameScientificTir is left from the first
// version, where only task 1.2 was played; it cannot be removed while the database
// holds results of students who played before the switch to the full practicum.
const (
	gamePraktika1     = "praktika-1"
	gamePraktika2     = "praktika-2"
	gameScientificTir = "nf-vs-nenf"
)

// gameAllowedIDs lists the games the server accepts. Without an allowlist anyone
// could dump garbage into the table under their own game name, and the cohort
// statistics would stop meaning anything.
var gameAllowedIDs = map[string]bool{gamePraktika1: true, gamePraktika2: true, gameScientificTir: true}

// gameResultLimits caps the input values. Not protection against an attacker
// (there is none here), but protection against typos and random garbage in the table.
const (
	gameMaxNameLen   = 60
	gameMaxAnswers   = 200
	gameMaxBodyBytes = 64 << 10
)

// gameAnswerIn is one student answer. Parsing is strict (decodeJSONStrict), so
// an extra field in the body gives a 400; the set must therefore match what
// game.js actually sends. The integration test catches exactly that: the first version did not
// know about src and rejected the real client's request entirely.
type gameAnswerIn struct {
	ID       int    `json:"id"`
	Lvl      int    `json:"lvl"`
	Src      string `json:"src"`
	Task     string `json:"task"`
	Correct  bool   `json:"correct"`
	TimedOut bool   `json:"timedOut"`
	MS       int    `json:"ms"`
}

// GameResult — POST /api/game/result.
func (h Handler) GameResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !h.guardGameCall(r.Context(), w, r) {
		return
	}
	var req struct {
		Game       string         `json:"game"`
		Family     string         `json:"family"`
		Name       string         `json:"name"`
		Group      string         `json:"group"`
		Score      int            `json:"score"`
		Correct    int            `json:"correct"`
		Total      int            `json:"total"`
		BestCombo  int            `json:"bestCombo"`
		LivesLeft  int            `json:"livesLeft"`
		DurationMS int64          `json:"durationMs"`
		Answers    []gameAnswerIn `json:"answers"`
		// The sessions in which the tasks were played. The numbers come from them, not from the
		// request body: a submitted score cannot be trusted; that is exactly what was
		// exploited by submitting 9,999,999 points.
		Sessions []string `json:"sessions"`
	}
	if err := decodeJSONStrict(w, r, gameMaxBodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}

	game := strings.TrimSpace(req.Game)
	if game == "" {
		game = gamePraktika1
	}
	if !gameAllowedIDs[game] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестная игра"})
		return
	}
	family, name, group := clipName(req.Family), clipName(req.Name), clipName(req.Group)
	if family == "" || name == "" || group == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужны фамилия, имя и группа"})
		return
	}
	if req.Total <= 0 || req.Correct < 0 || req.Correct > req.Total {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "результат не сходится"})
		return
	}

	// The score comes from the same student's closed sessions. The request body only
	// contributes what does not affect the grade (name, group); everything else
	// is recomputed here.
	if h.DB != nil {
		trusted, err := h.gameScoreFromSessions(r.Context(), req.Sessions, family, name, group)
		if err != nil {
			h.publicError(r.Context(), w, r, err)
			return
		}
		if trusted == nil {
			h.logSecurityEvent(r.Context(), "forged_score", "alert", r, map[string]any{
				"family": family, "group": group, "claimedScore": req.Score, "sessions": len(req.Sessions),
			})
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"ok": false, "error": "отчёт принимается только по сыгранным сессиям — пройдите задания в игре",
			})
			return
		}
		// A mismatch between the submitted and the real value is a trace of a tampering attempt.
		if req.Score != trusted.Score || req.Correct != trusted.Correct {
			h.logSecurityEvent(r.Context(), "forged_score", "warn", r, map[string]any{
				"family": family, "group": group,
				"claimed": map[string]any{"score": req.Score, "correct": req.Correct},
				"actual":  map[string]any{"score": trusted.Score, "correct": trusted.Correct},
			})
		}
		req.Score, req.Correct, req.Total = trusted.Score, trusted.Correct, trusted.Total
		req.DurationMS, req.LivesLeft = trusted.DurationMS, trusted.Armor
		answersBytes, _ := json.Marshal(trusted.Answers)
		var rebuilt []gameAnswerIn
		_ = json.Unmarshal(answersBytes, &rebuilt)
		req.Answers = rebuilt
	}
	if len(req.Answers) > gameMaxAnswers {
		req.Answers = req.Answers[:gameMaxAnswers]
	}
	answers, _ := json.Marshal(req.Answers)

	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	// The BEST result counts, not the first.
	//
	// A repeat submission used to be rejected entirely, as protection against replaying until
	// the desired number. But the teacher told the students that the best
	// attempt counts, and "first forever" directly contradicted that: an unlucky
	// restart buried a good game.
	//
	// There is no tampering here: the score still comes from the server's closed
	// sessions (above), and the client does not affect the number. Replaying the practicum is
	// exactly what the student is expected to do.
	var stored, attempts int
	err := h.DB.QueryRow(r.Context(), `
		insert into game_results
			(game, family, name, student_group, score, correct, total, best_combo, lives_left, duration_ms, answers, attempts)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1)
		on conflict (game, lower(trim(family)), lower(trim(name)), lower(trim(student_group)))
		do update set
			score       = greatest(game_results.score, excluded.score),
			correct     = case when excluded.score > game_results.score then excluded.correct else game_results.correct end,
			total       = case when excluded.score > game_results.score then excluded.total else game_results.total end,
			best_combo  = greatest(game_results.best_combo, excluded.best_combo),
			lives_left  = case when excluded.score > game_results.score then excluded.lives_left else game_results.lives_left end,
			duration_ms = case when excluded.score > game_results.score then excluded.duration_ms else game_results.duration_ms end,
			answers     = case when excluded.score > game_results.score then excluded.answers else game_results.answers end,
			attempts    = game_results.attempts + 1,
			created_at  = case when excluded.score > game_results.score then now() else game_results.created_at end
		returning score, attempts`,
		game, family, name, group,
		req.Score, req.Correct, req.Total, req.BestCombo, req.LivesLeft, req.DurationMS, answers).
		Scan(&stored, &attempts)
	if err != nil {
		h.publicError(r.Context(), w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "score": stored, "attempts": attempts, "improved": stored == req.Score,
	})
}

// GameResultReset is the dashboard's DELETE action: remove a result so the student
// can play again.
//
// It is needed because the practicum has never had authorisation: anyone can sign
// with someone else's full name, and there is one credit per signature. Closing this completely
// is only possible with accounts, which costs more than it gives for a seminar. So we give
// the teacher a cheap way to fix it: remove it, and the student replays.
func (h Handler) gameResultReset(ctx context.Context, w http.ResponseWriter, r *http.Request, family, name, group string) {
	family, name, group = clipName(family), clipName(name), clipName(group)
	if family == "" || group == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "нужны фамилия и группа"})
		return
	}
	tag, err := h.DB.Exec(ctx, `
		delete from game_results
		where lower(trim(family)) = lower(trim($1))
		  and ($2 = '' or lower(trim(name)) = lower(trim($2)))
		  and lower(trim(student_group)) = lower(trim($3))`, family, name, group)
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	h.writeAdminAudit(ctx, r, 0, "admin.game_result.reset", "game_results", nil,
		map[string]any{"family": family, "group": group, "removed": tag.RowsAffected()})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": tag.RowsAffected()})
}

// clipName brings a signature to a form fit for the table: no extra
// spaces and no wall of text instead of a surname.
func clipName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > gameMaxNameLen {
		s = strings.TrimSpace(s[:gameMaxNameLen])
	}
	return s
}

// GameStats handles GET /api/game/stats. A public summary for the cohort.
func (h Handler) GameStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	game := strings.TrimSpace(r.URL.Query().Get("game"))
	if game == "" {
		game = gamePraktika1
	}
	if !gameAllowedIDs[game] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестная игра"})
		return
	}
	ctx := r.Context()

	var players, games, topScore int
	var avgAccuracy *float64
	err := h.DB.QueryRow(ctx, `
		select count(distinct (lower(trim(family)), lower(trim(name)), lower(trim(student_group)))),
		       count(*),
		       coalesce(max(score), 0),
		       avg(case when total > 0 then correct::float / total end)
		from game_results where game = $1`, game).Scan(&players, &games, &topScore, &avgAccuracy)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.publicError(ctx, w, r, err)
		return
	}

	// The board with full names is students' personal data, and until now it was served
	// to anyone: first name, surname, group and result in one GET. The full
	// list is now available only with the teacher key; publicly, only initials.
	full := h.gameTeacherKeyOK(ctx, r)
	board, err := h.gameBoard(ctx, game)
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	if !full {
		for _, row := range board {
			row["family"] = maskName(row["family"])
			row["name"] = maskName(row["name"])
			delete(row, "group")
		}
	}
	hardest, err := h.gameHardest(ctx, game)
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"game":        game,
		"players":     players,
		"games":       games,
		"topScore":    topScore,
		"avgAccuracy": avgAccuracy,
		"board":       board,
		"hardest":     hardest,
	})
}

func (h Handler) gameBoard(ctx context.Context, game string) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, `
		select family, name, student_group, score, correct, total
		from game_results where game = $1
		order by score desc, duration_ms asc
		limit 20`, game)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var family, name, group string
		var score, correct, total int
		if err := rows.Scan(&family, &name, &group, &score, &correct, &total); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"family": family, "name": name, "group": group,
			"score": score, "correct": correct, "total": total,
		})
	}
	return out, rows.Err()
}

// gameHardest lists the statements that are failed most often.
//
// This list is the whole point: it shows the teacher which
// distinctions do not work for the group before the seminar. The threshold of three attempts
// cuts out chance: one mistake by one student must not put a
// statement into "the hardest".
func (h Handler) gameHardest(ctx context.Context, game string) ([]map[string]any, error) {
	rows, err := h.DB.Query(ctx, `
		select (a->>'id')::int as sid,
		       count(*) as seen,
		       avg(case when (a->>'correct')::boolean then 0 else 1 end) as err_rate
		from game_results g, jsonb_array_elements(g.answers) a
		where g.game = $1 and a ? 'id' and a ? 'correct'
		group by 1
		having count(*) >= 3 and avg(case when (a->>'correct')::boolean then 0 else 1 end) > 0.5
		order by err_rate desc, seen desc
		limit 10`, game)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, seen int
		var rate float64
		if err := rows.Scan(&id, &seen, &rate); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "seen": seen, "errorRate": rate})
	}
	return out, rows.Err()
}

// gameTeacherKeySetting is the system_settings key with which the teacher
// opens the list of submitted reports. The secret lives in the database, not in env or in
// code: changing it is one line of SQL or an edit in the admin UI, without a rebuild.
const gameTeacherKeySetting = "tir_teacher_key"

// gameResultsLimit caps the output. A cohort is dozens of people, not thousands;
// if there are more rows, the teacher narrows the query by group.
const gameResultsLimit = 500

// GameResults handles GET /api/game/results?key=...&game=...&group=..., the teacher's
// dashboard: all submitted reports with the answers to each statement.
//
// Authorisation is by a key in the request rather than a service session: the teacher has no
// Mindstrata account and should not need one, and the game lives on a separate domain where
// the service cookie does not go. The key comparison is constant-time.
func (h Handler) GameResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	ctx := r.Context()
	want, err := h.systemSetting(ctx, gameTeacherKeySetting)
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	if want == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "ключ преподавателя не задан на сервере"})
		return
	}
	if !h.gameTeacherKeyOK(ctx, r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "неверный ключ преподавателя"})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Action string `json:"action"`
			Family string `json:"family"`
			Name   string `json:"name"`
			Group  string `json:"group"`
		}
		if err := decodeJSONStrict(w, r, 8<<10, &req); err != nil || strings.TrimSpace(req.Action) != "reset" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "поддерживается только action=reset"})
			return
		}
		h.gameResultReset(ctx, w, r, req.Family, req.Name, req.Group)
		return
	}

	game := strings.TrimSpace(r.URL.Query().Get("game"))
	if game == "" {
		game = gamePraktika1
	}
	if !gameAllowedIDs[game] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестная игра"})
		return
	}
	group := clipName(r.URL.Query().Get("group"))

	rows, err := h.DB.Query(ctx, `
		select id, family, name, student_group, score, correct, total, best_combo, lives_left, duration_ms, answers, created_at, note
		from game_results
		where game = $1 and ($2 = '' or lower(trim(student_group)) = lower(trim($2)))
		order by created_at desc
		limit $3`, game, group, gameResultsLimit)
	if err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var family, name, sgroup string
		var score, correct, total, best, lives int
		var duration int64
		var answers json.RawMessage
		var created time.Time
		var note string
		if err := rows.Scan(&id, &family, &name, &sgroup, &score, &correct, &total, &best, &lives, &duration, &answers, &created, &note); err != nil {
			h.publicError(ctx, w, r, err)
			return
		}
		out = append(out, map[string]any{
			"id": id, "family": family, "name": name, "group": sgroup,
			"score": score, "correct": correct, "total": total,
			"bestCombo": best, "livesLeft": lives, "durationMs": duration,
			"answers": answers, "createdAt": created.UTC().Format(time.RFC3339), "note": note,
		})
	}
	if err := rows.Err(); err != nil {
		h.publicError(ctx, w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "game": game, "results": out})
}

// uuidPattern is exactly what postgres accepts for the uuid type.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// gameTrustedResult is what the server knows about the game by itself.
type gameTrustedResult struct {
	Score      int
	Correct    int
	Total      int
	Armor      int
	DurationMS int64
	Answers    []map[string]any
}

// gameScoreFromSessions assembles the total from the student's closed sessions.
//
// Sessions are checked for ownership: someone else's identifier will not work even
// if it was spotted, because the full name and group must match. Returns nil if
// there are no matching sessions; then the report is not accepted at all.
func (h Handler) gameScoreFromSessions(ctx context.Context, sessions []string, family, name, group string) (*gameTrustedResult, error) {
	if len(sessions) == 0 || len(sessions) > 8 {
		return nil, nil
	}
	// We check the format ourselves rather than relying on the driver's error text: the text depends on
	// the postgres version, and one day the check would silently stop working on it,
	// and a request with garbage would return 500 instead of a clear refusal.
	for _, id := range sessions {
		if !uuidPattern.MatchString(strings.TrimSpace(id)) {
			return nil, nil
		}
	}
	rows, err := h.DB.Query(ctx, `
		select task, score, correct, armor, answers,
		       extract(epoch from (coalesce(finished_at, last_seen_at) - started_at)) * 1000
		from game_sessions
		where id = any($1::uuid[])
		  and lower(trim(family)) = lower(trim($2))
		  and lower(trim(name)) = lower(trim($3))
		  and lower(trim(student_group)) = lower(trim($4))`,
		sessions, family, name, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &gameTrustedResult{}
	seenTasks := map[string]bool{}
	for rows.Next() {
		var task string
		var score, correct, armor int
		var answersRaw []byte
		var durationMS float64
		if err := rows.Scan(&task, &score, &correct, &armor, &answersRaw, &durationMS); err != nil {
			return nil, err
		}
		// One counted session per task: otherwise one could play 1.1 four
		// times and add up the points as if for the whole practicum.
		if seenTasks[task] {
			continue
		}
		seenTasks[task] = true

		var given []map[string]any
		_ = json.Unmarshal(answersRaw, &given)
		for _, a := range given {
			a["task"] = task
		}
		out.Answers = append(out.Answers, given...)
		out.Score += score
		out.Correct += correct
		out.Total += len(given)
		out.Armor += armor
		out.DurationMS += int64(durationMS)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out.Total == 0 {
		return nil, nil
	}
	if n := len(seenTasks); n > 0 {
		out.Armor /= n
	}
	return out, nil
}

// maskName keeps only the first letter of the name: the leaderboard should
// work ("there I am, third"), but a group list with results must not be
// publicly available.
func maskName(v any) string {
	s, _ := v.(string)
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return ""
	}
	return string(r[0]) + "."
}
