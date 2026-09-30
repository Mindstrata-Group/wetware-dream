//go:build integration

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// seedTasks puts enough tasks into the bank for a session to be built.
func seedTasks(t testing.TB, env *testsupport.Env, task string, answer any) {
	t.Helper()
	ctx := context.Background()
	for lvl := 1; lvl <= 3; lvl++ {
		for i := 0; i < 4; i++ {
			id := lvl*100 + i
			if _, err := env.Pool.Exec(ctx, `
				insert into game_tasks (game, task, ext_id, text, answer, lvl, src, why)
				values ('praktika-1', $1, $2, $3, $4::jsonb, $5, 'it', 'потому что так')`,
				task, id, fmt.Sprintf("Утверждение %d уровня %d", id, lvl), toJSON(answer), lvl); err != nil {
				t.Fatalf("сид заданий: %v", err)
			}
		}
	}
}

func toJSON(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", x)
	case string:
		return fmt.Sprintf("%q", x)
	}
	return "null"
}

func startSession(t testing.TB, ts *TestServer, task, family string) (string, []any) {
	t.Helper()
	code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "start", "task": task, "family": family, "name": "Тест Тестович", "group": "РИ-1",
	})
	if code != http.StatusOK {
		t.Fatalf("старт сессии: %d %+v", code, resp)
	}
	id, _ := resp["sessionId"].(string)
	items, _ := resp["items"].([]any)
	if id == "" || len(items) != 12 {
		t.Fatalf("сессия выдана неполной: id=%q заданий=%d", id, len(items))
	}
	return id, items
}

// TestSession_NeverLeaksAnswers: the main check. Students exploited exactly an
// answer leak: 48/48 at an average of 4.5 seconds per question.
// Tasks must arrive without the correct answer and without the explanation.
func TestSession_NeverLeaksAnswers(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	seedTasks(t, env, "1.1", 3)

	_, items := startSession(t, ts, "1.1", "Иванов")
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		for _, forbidden := range []string{"answer", "why", "cls", "nf", "truth", "marks"} {
			if _, found := it[forbidden]; found {
				t.Fatalf("в выданном задании поле %q — ответ утёк клиенту: %+v", forbidden, it)
			}
		}
		if it["t"] == nil || it["id"] == nil {
			t.Fatalf("задание пришло без текста или номера: %+v", it)
		}
	}

	// The full task list is no longer for students at all: the game takes tasks from
	// the session, and the bank is the teacher's work and a head start for anyone
	// preparing in advance.
	if code, _ := httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1", nil); code != http.StatusUnauthorized {
		t.Fatalf("банк заданий отдаётся без ключа: %d, want 401", code)
	}
}

// TestSession_ScoreIsServerSide: the second trick used was to send a score of
// 9 999 999. The report must take numbers from the session and ignore those sent.
func TestSession_ScoreIsServerSide(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "k")
	seedTasks(t, env, "1.1", 3)

	sid, items := startSession(t, ts, "1.1", "Петров")
	right := 0
	for i, raw := range items {
		it, _ := raw.(map[string]any)
		// Answer the first six correctly, miss the rest.
		value := 3
		if i >= 6 {
			value = 1
		}
		thinkABit(t, env, sid)
		code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid, "itemId": int(it["id"].(float64)), "value": value,
		})
		if code != http.StatusOK {
			t.Fatalf("ответ %d: %d %+v", i, code, resp)
		}
		if resp["correct"] == true {
			right++
		}
		// The server reveals the truth only after the answer: that is fine.
		if resp["truth"] == nil {
			t.Fatalf("сервер не вернул правильный ответ после ответа: %+v", resp)
		}
	}
	if right != 6 {
		t.Fatalf("сервер насчитал %d верных, ожидали 6", right)
	}

	code, fin := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid})
	if code != http.StatusOK {
		t.Fatalf("закрытие сессии: %d %+v", code, fin)
	}
	realScore := int(fin["score"].(float64))

	// Send a deliberately inflated report.
	code, resp := httpJSON(t, ts, "POST", "/api/game/result", map[string]any{
		"game": "praktika-1", "family": "Петров", "name": "Тест Тестович", "group": "РИ-1",
		"score": 9999999, "correct": 12, "total": 12, "bestCombo": 12, "livesLeft": 100,
		"durationMs": 1000, "answers": []any{}, "sessions": []string{sid},
	})
	if code != http.StatusOK {
		t.Fatalf("отправка отчёта: %d %+v", code, resp)
	}

	var storedScore, storedCorrect int
	if err := env.Pool.QueryRow(context.Background(),
		`select score, correct from game_results where family = 'Петров'`).Scan(&storedScore, &storedCorrect); err != nil {
		t.Fatalf("чтение результата: %v", err)
	}
	if storedScore == 9999999 {
		t.Fatal("накрученный счёт попал в таблицу — защита не работает")
	}
	if storedScore != realScore || storedCorrect != 6 {
		t.Fatalf("в базе счёт %d и %d верных, а сервер насчитал %d и 6", storedScore, storedCorrect, realScore)
	}

	// The substitution must leave a trace.
	var forged int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from security_events where kind = 'forged_score'`).Scan(&forged); err != nil {
		t.Fatalf("журнал: %v", err)
	}
	if forged == 0 {
		t.Fatal("подмена счёта не попала в журнал безопасности")
	}
}

// TestSession_ResultWithoutSessionRejected: a report "out of thin air", without a
// single played session, must not be accepted; otherwise the client draws the
// score again.
func TestSession_ResultWithoutSessionRejected(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	body := map[string]any{
		"game": "praktika-1", "family": "Хакеров", "name": "Хакер", "group": "РИ-9",
		"score": 999999, "correct": 48, "total": 48, "bestCombo": 12, "livesLeft": 100,
		"durationMs": 1000, "answers": []any{},
	}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusBadRequest {
		t.Fatalf("отчёт без сессий: %d %+v, want 400", code, resp)
	}
	body["sessions"] = []string{"00000000-0000-0000-0000-000000000000"}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusBadRequest {
		t.Fatalf("отчёт с выдуманной сессией: %d, want 400", code)
	}
	body["sessions"] = []string{"не-uuid-вовсе"}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusBadRequest {
		t.Fatalf("отчёт с мусором вместо сессии: %d, want 400", code)
	}

	var n int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from game_results`).Scan(&n); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if n != 0 {
		t.Fatalf("в таблице %d записей — отчёт без сессии всё-таки прошёл", n)
	}
}

// TestSession_AlienSessionRejected: someone else's session cannot be claimed
// even knowing its id: full name and group must match.
func TestSession_AlienSessionRejected(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	seedTasks(t, env, "1.1", 3)

	sid, items := startSession(t, ts, "1.1", "Честный")
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid, "itemId": int(it["id"].(float64)), "value": 3,
		})
	}
	httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid})

	code, resp := httpJSON(t, ts, "POST", "/api/game/result", map[string]any{
		"game": "praktika-1", "family": "Чужой", "name": "Чужой", "group": "РИ-1",
		"score": 100, "correct": 12, "total": 12, "bestCombo": 1, "livesLeft": 100,
		"durationMs": 1000, "answers": []any{}, "sessions": []string{sid},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("присвоение чужой сессии: %d %+v, want 400", code, resp)
	}
}

// TestSession_AnswersGoInOrderOnce: answers are accepted strictly in order and
// once each; otherwise a miss could be "re-answered" and answers sent in bulk.
func TestSession_AnswersGoInOrderOnce(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	seedTasks(t, env, "1.2", true)

	sid, items := startSession(t, ts, "1.2", "Порядков")
	first := int(items[0].(map[string]any)["id"].(float64))
	second := int(items[1].(map[string]any)["id"].(float64))

	// An answer to the wrong task.
	if code, _ := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": second, "value": true,
	}); code != http.StatusBadRequest {
		t.Fatalf("ответ не по порядку прошёл: %d", code)
	}
	// Correct order.
	thinkABit(t, env, sid)
	code, ok := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": first, "value": true,
	})
	if code != http.StatusOK || ok["correct"] != true {
		t.Fatalf("ответ по порядку не прошёл: %d %+v", code, ok)
	}
	// Repeating the same task is accepted, but the verdict stays the same: the
	// request may have arrived while the server's response was lost, and it must not
	// be rejected. Yet the result cannot be rewritten by a repeat either: here we
	// send a deliberately different value and expect the previous verdict.
	code, again := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": first, "value": false,
	})
	if code != http.StatusOK || again["replayed"] != true {
		t.Fatalf("повтор после потерянного ответа: %d %+v", code, again)
	}
	if again["correct"] != true {
		t.Fatalf("повтор переписал верный ответ на неверный: %+v", again)
	}

	// Client desync is its own kind of record: real attempts to break into someone
	// else's game must not get lost among them.
	var desync int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from security_events where kind = 'out_of_order'`).Scan(&desync); err != nil {
		t.Fatalf("журнал: %v", err)
	}
	if desync == 0 {
		t.Fatal("попытка ответить не на то задание не попала в журнал")
	}
}

// TestSession_BadTeacherKeyIsLogged: brute-forcing the key must leave a trace;
// otherwise it is discovered only after the fact and not from the system.
func TestSession_BadTeacherKeyIsLogged(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "правильный-ключ")

	if code, _ := httpJSON(t, ts, "GET", "/api/game/results?key=подбор", nil); code != http.StatusUnauthorized {
		t.Fatalf("чужой ключ: %d, want 401", code)
	}
	var bad int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from security_events where kind = 'bad_key'`).Scan(&bad); err != nil {
		t.Fatalf("журнал: %v", err)
	}
	if bad == 0 {
		t.Fatal("неудачная попытка ключа не попала в журнал безопасности")
	}
}

// TestPublicStatsHidesNames: the leaderboard must not be a group list with
// results. Publicly: initials without the group; in full only with the key.
func TestPublicStatsHidesNames(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "kk")
	seedTasks(t, env, "1.1", 3)

	sid, items := startSession(t, ts, "1.1", "Фамилия")
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid, "itemId": int(it["id"].(float64)), "value": 3,
		})
	}
	_, fin := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid})
	body := gameBody("Фамилия", "Тест Тестович", "РИ-1",
		int(fin["score"].(float64)), int(fin["correct"].(float64)), 12, nil)
	body["sessions"] = []string{sid}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusOK {
		t.Fatalf("отчёт не принят: %d %+v", code, resp)
	}

	_, pub := httpJSON(t, ts, "GET", "/api/game/stats", nil)
	board, _ := pub["board"].([]any)
	if len(board) == 0 {
		t.Fatal("публичная доска пуста")
	}
	row, _ := board[0].(map[string]any)
	if row["family"] == "Фамилия" || row["name"] == "Тест Тестович" {
		t.Fatalf("публично видно полное ФИО студента: %+v", row)
	}
	if _, hasGroup := row["group"]; hasGroup {
		t.Fatalf("публично видна группа студента: %+v", row)
	}
	if row["family"] != "Ф." {
		t.Fatalf("ожидали инициал, получили %v", row["family"])
	}
}

// TestTaskBankNeedsKey: the whole bank of statements is the teacher's work and a
// head start for anyone preparing in advance. We do not serve it without the key.
func TestTaskBankNeedsKey(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "kkk")
	seedTasks(t, env, "1.3", "rnd")

	if code, _ := httpJSON(t, ts, "GET", "/api/game/tasks?task=1.3", nil); code != http.StatusUnauthorized {
		t.Fatalf("банк без ключа: %d, want 401", code)
	}
	code, resp := httpJSON(t, ts, "GET", "/api/game/tasks?task=1.3&key=kkk", nil)
	if code != http.StatusOK {
		t.Fatalf("банк с ключом: %d %+v", code, resp)
	}
	if items, _ := resp["items"].([]any); len(items) != 12 {
		t.Fatalf("с ключом пришло %d записей, want 12", len(items))
	}
}

// TestResultResetAllowsReplay: anyone can sign with someone else's full name:
// the practicum has no auth. The teacher must be able to remove a result;
// otherwise a taken signature blocks the student forever.
func TestResultResetAllowsReplay(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "k4")
	seedTasks(t, env, "1.1", 3)

	sid, score, correct := 0, 0, 0
	_ = sid
	s1, items := startSession(t, ts, "1.1", "Занятов")
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": s1, "itemId": int(it["id"].(float64)), "value": 3,
		})
	}
	_, fin := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": s1})
	score, correct = int(fin["score"].(float64)), int(fin["correct"].(float64))
	body := gameBody("Занятов", "Тест Тестович", "РИ-1", score, correct, 12, nil)
	body["sessions"] = []string{s1}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusOK {
		t.Fatal("первый отчёт не принят")
	}
	// A replay is accepted and counted as a second attempt: the best one counts.
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusOK || resp["attempts"] != float64(2) {
		t.Fatalf("повтор: %d %+v, ждали 200 и вторую попытку", code, resp)
	}
	// Cannot remove it without the key.
	if code, _ := httpJSON(t, ts, "POST", "/api/game/results", map[string]any{
		"action": "reset", "family": "Занятов", "name": "Тест Тестович", "group": "РИ-1",
	}); code != http.StatusUnauthorized {
		t.Fatalf("снятие без ключа: %d, want 401", code)
	}
	// With the key it can be removed, and the student plays again.
	code, resp := httpJSON(t, ts, "POST", "/api/game/results?key=k4", map[string]any{
		"action": "reset", "family": "Занятов", "name": "Тест Тестович", "group": "РИ-1",
	})
	if code != http.StatusOK || resp["removed"] != float64(1) {
		t.Fatalf("снятие с ключом: %d %+v", code, resp)
	}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusOK {
		t.Fatal("после снятия отчёт снова не принимается")
	}
}

// TestSession_HourlyCapLetsStudentReplay: the sessions-per-hour cap exists
// against score farming, but on 23.09 four students hit it right in the seminar:
// every page reload started a new session, and the limit burned out in half an
// hour. The test pins two requirements: normal replaying has plenty of room, and
// farming still hits the wall.
func TestSession_HourlyCapLetsStudentReplay(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	seedTasks(t, env, "1.1", 3)

	// Exactly the session cap must pass: these are legitimate replays.
	for i := 0; i < gameMaxSessionsHour; i++ {
		code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "start", "task": "1.1", "family": "Переигров",
			"name": "Тест Тестович", "group": "РИ-1",
		})
		if code != http.StatusOK {
			t.Fatalf("попытка %d из %d отбита: %d %+v", i+1, gameMaxSessionsHour, code, resp)
		}
	}

	// The next one is farming.
	if code, _ := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "start", "task": "1.1", "family": "Переигров",
		"name": "Тест Тестович", "group": "РИ-1",
	}); code != http.StatusTooManyRequests {
		t.Fatalf("сверх потолка: %d, want 429", code)
	}

	// The cap is counted per student, not for everyone at once: a classmate must
	// not suffer for someone else's farming.
	if code, _ := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "start", "task": "1.1", "family": "Соседов",
		"name": "Тест Тестович", "group": "РИ-1",
	}); code != http.StatusOK {
		t.Fatalf("соседа задело чужим потолком: %d", code)
	}

	// Headroom over a real seminar: on 23.09 the record was 12 sessions per hour.
	if gameMaxSessionsHour < 30 {
		t.Fatalf("потолок %d — мало для живого семинара", gameMaxSessionsHour)
	}
}

// TestSession_RestoreReturnsBestRun: progress survives a page reload.
//
// On 23.09 students lost their progress on every F5 and replayed in circles.
// The server knew the result all along: it checks the answers itself. We check
// two promises: restore returns the BEST attempt (the teacher counts the best
// run), and someone else's is not returned.
func TestSession_RestoreReturnsBestRun(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	// A weak game fails the third level, a good one has no mistakes at all.
	weakSid, weakScore, _ := playSession(t, env, ts, "1.1", "Забывцев", "Тест Тестович", "РИ-1", 3)
	goodSid, goodScore, goodCorrect := playSession(t, env, ts, "1.1", "Забывцев", "Тест Тестович", "РИ-1", 0)
	if goodScore <= weakScore {
		t.Fatalf("тест бессмысленен: %d не лучше %d", goodScore, weakScore)
	}

	code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "restore", "family": "Забывцев", "name": "Тест Тестович", "group": "РИ-1",
	})
	if code != http.StatusOK {
		t.Fatalf("восстановление: %d %+v", code, resp)
	}
	best, _ := resp["best"].([]any)
	if len(best) != 1 {
		t.Fatalf("заданий восстановлено %d, играли одно: %+v", len(best), resp)
	}
	b, _ := best[0].(map[string]any)
	if b["sessionId"] != goodSid {
		t.Fatalf("вернулась сессия %v, а лучшая была %s (слабая %s)", b["sessionId"], goodSid, weakSid)
	}
	if int(b["score"].(float64)) != goodScore || int(b["correct"].(float64)) != goodCorrect {
		t.Fatalf("счёт восстановлен неверно: %+v, ждали score=%d correct=%d", b, goodScore, goodCorrect)
	}

	// The breakdown must come back in full: the student fills in the report form
	// from it per the manual. An empty table in the downloaded report means the
	// practice is not passed.
	ans, _ := b["answers"].([]any)
	if len(ans) != 12 {
		t.Fatalf("ответов восстановлено %d из 12: без них отчёт сдавать нечем", len(ans))
	}
	for i, raw := range ans {
		a, _ := raw.(map[string]any)
		for _, need := range []string{"t", "why", "truth", "given", "correct"} {
			if a[need] == nil {
				t.Fatalf("в ответе %d нет поля %q — колонка отчёта останется пустой: %+v", i, need, a)
			}
		}
	}

	// Someone else's full name does not open someone else's progress.
	_, alien := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "restore", "family": "Чужаков", "name": "Тест Тестович", "group": "РИ-1",
	})
	if got, _ := alien["best"].([]any); len(got) != 0 {
		t.Fatalf("чужому отдали %d прохождений: %+v", len(got), alien)
	}
}

// TestSession_RestoreFixatesAbandonedRun: an abandoned game is not lost.
//
// The score is written on every answer, but without the closing mark nobody sees
// the session: neither restore nor the report. That is how a 27-point game was
// lost by a student who closed the tab on 23.09. Now entering finishes it for them.
func TestSession_RestoreFixatesAbandonedRun(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()
	seedTasks(t, env, "1.1", 3)

	// Play half of the tasks and "close the tab": finish is not called.
	sid, items := startSession(t, ts, "1.1", "Броскин")
	for i := 0; i < 5; i++ {
		it, _ := items[i].(map[string]any)
		httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid, "itemId": int(it["id"].(float64)), "value": 3,
		})
	}
	// Push the last activity back past a minute of silence, as if the student left.
	if _, err := env.Pool.Exec(ctx,
		`update game_sessions set last_seen_at = now() - interval '5 minutes' where id = $1::uuid`, sid); err != nil {
		t.Fatalf("подготовка: %v", err)
	}

	code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "restore", "family": "Броскин", "name": "Тест Тестович", "group": "РИ-1",
	})
	if code != http.StatusOK {
		t.Fatalf("восстановление: %d %+v", code, resp)
	}
	best, _ := resp["best"].([]any)
	if len(best) != 1 {
		t.Fatalf("брошенная игра пропала: %+v", resp)
	}
	b, _ := best[0].(map[string]any)
	if int(b["total"].(float64)) != 5 {
		t.Fatalf("ответов восстановлено %v, дали 5: %+v", b["total"], b)
	}

	// Closed at the time of the last answer, not now(): otherwise the abandoned game
	// would be credited with the hours the student spent off the page.
	var minutes float64
	if err := env.Pool.QueryRow(ctx,
		`select extract(epoch from (now() - finished_at))/60 from game_sessions where id = $1::uuid`, sid).
		Scan(&minutes); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if minutes < 4 {
		t.Fatalf("сессия закрыта текущим временем (%.1f мин назад), а не временем ухода", minutes)
	}

	// A game in progress right now is not closed: its last_seen_at is fresh.
	liveSid, liveItems := startSession(t, ts, "1.1", "Играев")
	it, _ := liveItems[0].(map[string]any)
	httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": liveSid, "itemId": int(it["id"].(float64)), "value": 3,
	})
	httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "restore", "family": "Играев", "name": "Тест Тестович", "group": "РИ-1",
	})
	var open bool
	if err := env.Pool.QueryRow(ctx,
		`select finished_at is null from game_sessions where id = $1::uuid`, liveSid).Scan(&open); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if !open {
		t.Fatal("восстановление закрыло игру, которая идёт прямо сейчас")
	}
}

// TestSession_TooFastAnswerNotCounted: a scripted answer earns no points.
//
// On 23.09 a student left a session open for 17 minutes and then sent 11 answers
// in 1.4 seconds, 125–141 ms each: eight correct, all with different options.
// The server was not hacked: no answers leaked from it. The statement TEXTS are
// bound to leak: they are collected over several sessions and worked out in
// advance. For a multiple-choice format the only remaining protection is time.
func TestSession_TooFastAnswerNotCounted(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()
	seedTasks(t, env, "1.1", 3)

	sid, items := startSession(t, ts, "1.1", "Скриптов")

	// Answer CORRECTLY, but instantly. The server measures time from the previous
	// answer, so before each attempt we move the activity mark to "now" and wait for
	// the request to fit under the threshold. Under parallel load the round trip
	// itself can take half a second, and then this would test the machine's speed
	// rather than the rule, so we try on several tasks in a row.
	var resp map[string]any
	caught := false
	used := 0
	for i := 0; i < len(items) && !caught; i++ {
		used = i + 1
		it, _ := items[i].(map[string]any)
		if _, err := env.Pool.Exec(ctx,
			`update game_sessions set last_seen_at = now() where id = $1::uuid`, sid); err != nil {
			t.Fatalf("подготовка: %v", err)
		}
		var code int
		code, resp = httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid, "itemId": int(it["id"].(float64)), "value": 3,
		})
		if code != http.StatusOK {
			t.Fatalf("ответ: %d %+v", code, resp)
		}
		if ms, ok := resp["ms"].(float64); ok && int(ms) < 250 {
			caught = true
		}
	}
	if !caught {
		t.Skip("машина слишком загружена: ни один запрос не уложился в 250 мс")
	}
	if resp["tooFast"] != true {
		t.Fatalf("сервер не распознал подстановку: %+v", resp)
	}
	if resp["correct"] == true {
		t.Fatalf("верный по содержанию, но подставленный ответ засчитан: %+v", resp)
	}
	if int(resp["score"].(float64)) != 0 {
		t.Fatalf("за подставленный ответ начислено %v очков", resp["score"])
	}

	// A trace must remain, and specifically as an alert: the teacher needs to see it,
	// not dig for it among routine records.
	var n int
	if err := env.Pool.QueryRow(ctx,
		`select count(*) from security_events where kind='impossible_timing' and severity='alert'`).Scan(&n); err != nil {
		t.Fatalf("журнал: %v", err)
	}
	if n == 0 {
		t.Fatal("подстановка не попала в журнал тревогой")
	}

	// A person who read the prompt is not penalised: an answer after a pause goes
	// through as usual. Take the next unanswered task and push the activity mark
	// back, as if the student thought for three seconds.
	if used >= len(items) {
		t.Skip("все задания ушли на поиск быстрого ответа")
	}
	it2, _ := items[used].(map[string]any)
	if _, err := env.Pool.Exec(ctx,
		`update game_sessions set last_seen_at = now() - interval '3 seconds' where id = $1::uuid`, sid); err != nil {
		t.Fatalf("подготовка: %v", err)
	}
	_, resp2 := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": int(it2["id"].(float64)), "value": 3,
	})
	if resp2["tooFast"] == true || resp2["correct"] != true {
		t.Fatalf("честный ответ после раздумья не засчитан: %+v", resp2)
	}
}

// TestSession_ReplayAfterLostResponse: repeating an answer does not break the game.
//
// On 23.09 one student got "answer to the wrong task" four times in a row: the
// request arrived, the server's response was lost, the client offered to answer
// again, and the server rejected the repeat because it had already closed the
// task. The person restarted the whole task, burning attempts.
func TestSession_ReplayAfterLostResponse(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	seedTasks(t, env, "1.1", 3)

	sid, items := startSession(t, ts, "1.1", "Обрывов")
	it, _ := items[0].(map[string]any)
	id := int(it["id"].(float64))

	thinkABit(t, env, sid)
	_, first := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": id, "value": 3,
	})
	if first["correct"] != true {
		t.Fatalf("первый ответ: %+v", first)
	}

	// The same answer again, as after a lost server response.
	code, again := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": id, "value": 3,
	})
	if code != http.StatusOK {
		t.Fatalf("повтор отвергнут: %d %+v", code, again)
	}
	if again["replayed"] != true || again["correct"] != first["correct"] {
		t.Fatalf("повтор вернул не прежний вердикт: %+v", again)
	}
	if again["score"] != first["score"] {
		t.Fatalf("повтор изменил счёт: было %v, стало %v", first["score"], again["score"])
	}

	// A miss cannot be rewritten by a repeat: the verdict is always the same.
	it2, _ := items[1].(map[string]any)
	id2 := int(it2["id"].(float64))
	thinkABit(t, env, sid)
	httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": id2, "value": 1, // miss
	})
	_, retry := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": id2, "value": 3, // now correct
	})
	if retry["correct"] == true {
		t.Fatalf("промах переписан повтором с верным ответом: %+v", retry)
	}
}

// TestSession_UniformPaceFlagged: an even pace gives away a game played from a
// prepared list.
//
// The 250 ms threshold is bypassed by a pause in the script, but an even pace is
// not. Real students' answer times vary several-fold; the script on 23.09 had
// every answer within 125–141 ms.
func TestSession_UniformPaceFlagged(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()
	seedTasks(t, env, "1.1", 3)

	// Even pace: each answer exactly 5 seconds apart. The base is generous so that
	// jitter of the request itself does not create spread for us; otherwise the test
	// would measure the network, not the rule.
	sid, items := startSession(t, ts, "1.1", "Ровнов")
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		if _, err := env.Pool.Exec(ctx,
			`update game_sessions set last_seen_at = now() - interval '5 seconds' where id = $1::uuid`, sid); err != nil {
			t.Fatalf("подготовка: %v", err)
		}
		httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid, "itemId": int(it["id"].(float64)), "value": 3,
		})
	}
	httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid})

	var flagged int
	if err := env.Pool.QueryRow(ctx,
		`select count(*) from security_events where kind='uniform_pace'`).Scan(&flagged); err != nil {
		t.Fatalf("журнал: %v", err)
	}
	if flagged == 0 {
		t.Fatal("ровный темп не отмечен — скрипт с паузой пройдёт незамеченным")
	}

	// A real player: times vary. They must not be flagged.
	sid2, items2 := startSession(t, ts, "1.1", "Живов")
	pauses := []int{900, 4200, 1700, 8300, 2600, 12000, 3100, 5400, 1200, 7700, 2200, 3900}
	for i, raw := range items2 {
		it, _ := raw.(map[string]any)
		if _, err := env.Pool.Exec(ctx,
			`update game_sessions set last_seen_at = now() - make_interval(secs => $2::numeric/1000) where id = $1::uuid`,
			sid2, pauses[i]); err != nil {
			t.Fatalf("подготовка: %v", err)
		}
		httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid2, "itemId": int(it["id"].(float64)), "value": 3,
		})
	}
	httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid2})

	var onLive int
	if err := env.Pool.QueryRow(ctx,
		`select count(*) from security_events where kind='uniform_pace' and detail->>'family'='Живов'`).Scan(&onLive); err != nil {
		t.Fatalf("журнал: %v", err)
	}
	if onLive != 0 {
		t.Fatal("живой игрок с рваным темпом отмечен как скрипт — ложная тревога")
	}
}
