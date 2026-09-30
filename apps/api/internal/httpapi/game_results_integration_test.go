//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func gameBody(family, name, group string, score, correct, total int, answers []map[string]any) map[string]any {
	return map[string]any{
		"game": "praktika-1", "family": family, "name": name, "group": group,
		"score": score, "correct": correct, "total": total,
		"bestCombo": 3, "livesLeft": 2, "durationMs": 120000,
		"answers": answers,
	}
}

// playSession plays a task through and returns the session id and what the
// server counted. A report is now accepted only for played sessions: that is
// the protection against a submitted score, so the tests really play.
// wrongOnLevel: the level at which to answer wrongly (0: answer right everywhere).
// A level, not "the first N": the server serves the task set in random order,
// and "the first N" would differ between students, so statistics on hard
// statements could not be checked that way.
// thinkABit pushes back the last-activity mark, imitating a person who read the
// statement. Without it the server treats the answer as scripted (faster than
// gameMinPlausibleMS) and does not count it: the rule appeared after 23.09, when
// a student sent 11 answers in 1.4 seconds.
func thinkABit(t testing.TB, env *testsupport.Env, sessionID string) {
	t.Helper()
	if _, err := env.Pool.Exec(context.Background(),
		`update game_sessions set last_seen_at = now() - interval '3 seconds' where id = $1::uuid`,
		sessionID); err != nil {
		t.Fatalf("пауза на раздумье: %v", err)
	}
}

func playSession(t testing.TB, env *testsupport.Env, ts *TestServer, task, family, name, group string, wrongOnLevel int) (string, int, int) {
	t.Helper()
	seedTasksOnce(t, env, task)
	code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "start", "task": task, "family": family, "name": name, "group": group,
	})
	if code != http.StatusOK {
		t.Fatalf("старт сессии: %d %+v", code, resp)
	}
	sid, _ := resp["sessionId"].(string)
	items, _ := resp["items"].([]any)
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		value := 3 // correct answer in the seeded bank
		if wrongOnLevel > 0 && int(it["lvl"].(float64)) == wrongOnLevel {
			value = 1
		}
		thinkABit(t, env, sid)
		httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
			"action": "answer", "sessionId": sid, "itemId": int(it["id"].(float64)), "value": value,
		})
	}
	code, fin := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid})
	if code != http.StatusOK {
		t.Fatalf("закрытие сессии: %d %+v", code, fin)
	}
	return sid, int(fin["score"].(float64)), int(fin["correct"].(float64))
}

// seedTasksOnce fills the bank if it is empty: a session cannot be built from nothing.
func seedTasksOnce(t testing.TB, env *testsupport.Env, task string) {
	t.Helper()
	var n int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from game_tasks where task = $1`, task).Scan(&n); err != nil {
		t.Fatalf("проверка банка: %v", err)
	}
	if n >= 12 {
		return
	}
	seedTasks(t, env, task, 3)
}

// TestGameResult_KeepsBestAttempt: the BEST attempt counts, not the first.
//
// The teacher told students the best run counts, so a successful retry must
// overwrite a weak one, and a weak one must not overwrite a good one. There is
// still a single row, and the number of attempts is visible.
func TestGameResult_KeepsBestAttempt(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()

	// First game is weak: fail the whole third level, the most valuable one.
	weakSid, weakScore, weakCorrect := playSession(t, env, ts, "1.1", "Иванов", "Иван", "РИ-410001", 3)
	weak := gameBody("Иванов", "Иван", "РИ-410001", weakScore, weakCorrect, 12, nil)
	weak["sessions"] = []string{weakSid}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", weak); code != http.StatusOK {
		t.Fatalf("первая отправка: %d body=%+v", code, resp)
	}

	// Second game is good. It used to be rejected with 409, and the student was
	// stuck with the weak result forever.
	goodSid, goodScore, goodCorrect := playSession(t, env, ts, "1.1", "Иванов", "Иван", "РИ-410001", 0)
	if goodScore <= weakScore {
		t.Fatalf("тест бессмысленен: удачная игра %d не лучше слабой %d", goodScore, weakScore)
	}
	good := gameBody("Иванов", "Иван", "РИ-410001", goodScore, goodCorrect, 12, nil)
	good["sessions"] = []string{goodSid}
	code, resp := httpJSON(t, ts, "POST", "/api/game/result", good)
	if code != http.StatusOK {
		t.Fatalf("удачный повтор отклонён: %d body=%+v", code, resp)
	}
	if resp["improved"] != true {
		t.Fatalf("сервер не сообщил об улучшении: %+v", resp)
	}

	// Third game is weak again. It must NOT spoil the counted result.
	againSid, againScore, againCorrect := playSession(t, env, ts, "1.1", "Иванов", "Иван", "РИ-410001", 3)
	again := gameBody("Иванов", "Иван", "РИ-410001", againScore, againCorrect, 12, nil)
	again["sessions"] = []string{againSid}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", again); code != http.StatusOK {
		t.Fatalf("третья отправка: %d %+v", code, resp)
	}

	var n, storedScore, attempts int
	if err := env.Pool.QueryRow(ctx,
		`select count(*), coalesce(max(score),0), coalesce(max(attempts),0)
		   from game_results where family='Иванов'`).Scan(&n, &storedScore, &attempts); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if n != 1 {
		t.Fatalf("в базе %d строк, want одну", n)
	}
	if storedScore != goodScore {
		t.Fatalf("зачтено %d, а лучшая игра была %d", storedScore, goodScore)
	}
	if attempts != 3 {
		t.Fatalf("попыток учтено %d, сыграно 3", attempts)
	}
}

// TestGameResult_NormalizesSignature: "  ivanov  " and "Ivanov" (in Cyrillic)
// are the same student. Without normalisation a resubmission with an extra space
// would bypass the protection, and the group statistics would drift.
func TestGameResult_NormalizesSignature(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	sid, score, correct := playSession(t, env, ts, "1.1", "Петров", "Пётр", "РИ-410002", 3)
	first := gameBody("Петров", "Пётр", "РИ-410002", score, correct, 12, nil)
	first["sessions"] = []string{sid}
	code, resp := httpJSON(t, ts, "POST", "/api/game/result", first)
	if code != http.StatusOK {
		t.Fatalf("первая отправка: %d %+v", code, resp)
	}
	// The same signature in another case and with spaces is the same student, not a
	// new one. Without normalisation the table would get a second row, and one
	// person would appear twice in the ranking.
	sid2, score2, correct2 := playSession(t, env, ts, "1.1", "  петров ", "  ПЁТР", " ри-410002  ", 0)
	second := gameBody("  петров ", "  ПЁТР", " ри-410002  ", score2, correct2, 12, nil)
	second["sessions"] = []string{sid2}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", second); code != http.StatusOK {
		t.Fatalf("та же подпись с пробелами: %d %+v", code, resp)
	}
	var n, attempts int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*), coalesce(max(attempts),0) from game_results
		  where lower(trim(family))='петров'`).Scan(&n, &attempts); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if n != 1 || attempts != 2 {
		t.Fatalf("строк %d, попыток %d — «  петров  » и «Петров» разъехались", n, attempts)
	}
}

// TestGameResult_RejectsBrokenInput: the server must not accept a result that is
// inconsistent with itself, nor an unknown game.
func TestGameResult_RejectsBrokenInput(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"верных больше, чем всего", gameBody("Сидоров", "Сидор", "РИ-1", 100, 25, 20, nil)},
		{"нет группы", gameBody("Сидоров", "Сидор", "", 100, 10, 20, nil)},
		{"пустая фамилия", gameBody("   ", "Сидор", "РИ-1", 100, 10, 20, nil)},
		{"всего нуль целей", gameBody("Сидоров", "Сидор", "РИ-1", 100, 0, 0, nil)},
	}
	for _, c := range cases {
		code, resp := httpJSON(t, ts, "POST", "/api/game/result", c.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d body=%+v, want 400", c.name, code, resp)
		}
	}

	bad := gameBody("Сидоров", "Сидор", "РИ-1", 100, 10, 20, nil)
	bad["game"] = "чужая-игра"
	if code, _ := httpJSON(t, ts, "POST", "/api/game/result", bad); code != http.StatusBadRequest {
		t.Errorf("незнакомая игра: %d, want 400", code)
	}

	// The first version's identifier is still accepted: the database holds results
	// of students who played before the switch to the full practice, and dropping it
	// would lose their statistics.
	lsid, lscore, lcorrect := playSession(t, env, ts, "1.1", "Легасин", "Лег", "РИ-9", 3)
	legacy := gameBody("Легасин", "Лег", "РИ-9", lscore, lcorrect, 12, nil)
	legacy["game"] = "nf-vs-nenf"
	legacy["sessions"] = []string{lsid}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", legacy); code != http.StatusOK {
		t.Errorf("легаси-идентификатор игры: %d body=%+v, want 200", code, resp)
	}
}

// TestGameStats_CountsAndHardest: statistics count the group and surface the
// statements most people fail. The game was made for this list: it shows what
// to go through in the seminar.
func TestGameStats_CountsAndHardest(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	// Statement 42 is failed by all three, statement 7 by nobody.
	for _, fam := range []string{"Аникин", "Борисов", "Волков"} {
		// All three fail third-level tasks: exactly those must show up in the
		// "most often failed" summary.
		ssid, sscore, scorrect := playSession(t, env, ts, "1.1", fam, "Студент", "РИ-410003", 3)
		body := gameBody(fam, "Студент", "РИ-410003", sscore, scorrect, 12, nil)
		body["sessions"] = []string{ssid}
		if code, resp := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusOK {
			t.Fatalf("отправка %s: %d body=%+v", fam, code, resp)
		}
	}

	code, resp := httpJSON(t, ts, "GET", "/api/game/stats", nil)
	if code != http.StatusOK {
		t.Fatalf("статистика: %d body=%+v", code, resp)
	}
	if got := int(resp["players"].(float64)); got != 3 {
		t.Errorf("игроков = %d, want 3", got)
	}
	// The server computes the score, so we check not a specific number but that a
	// score was earned at all: a hard number here would only test the scoring formula.
	if got := int(resp["topScore"].(float64)); got <= 0 {
		t.Errorf("лучший результат = %d, ожидали положительный", got)
	}

	hardest, _ := resp["hardest"].([]any)
	if len(hardest) == 0 {
		t.Fatal("список трудных утверждений пуст, хотя девять заданий провалили все трое")
	}
	// The server serves the task set at random, so we check a property, not a
	// specific number: every entry in the hard list must have three attempts and an
	// error rate above one half.
	for _, raw := range hardest {
		h, _ := raw.(map[string]any)
		if int(h["seen"].(float64)) < 3 {
			t.Errorf("в списке трудных запись с %v попытками, порог — три", h["seen"])
		}
		if h["errorRate"].(float64) <= 0.5 {
			t.Errorf("в списке трудных запись с долей ошибок %v, ожидали больше половины", h["errorRate"])
		}
	}
}

// TestGameResults_TeacherKey: the teacher's cabinet opens only with the key from
// system_settings. With no key in the DB: 503 (not 401, otherwise the teacher
// would think they mistyped the key when none was set up); a wrong key: 401;
// the right key: a list with answers per statement.
func TestGameResults_TeacherKey(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	ctx := context.Background()

	tsid, tscore, tcorrect := playSession(t, env, ts, "1.1", "Сидоров", "Пётр", "РИ-410002", 3)
	body := gameBody("Сидоров", "Пётр", "РИ-410002", tscore, tcorrect, 12, nil)
	body["sessions"] = []string{tsid}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/result", body); code != http.StatusOK {
		t.Fatalf("отправка: %d body=%+v", code, resp)
	}

	_, _ = env.Pool.Exec(ctx, `delete from system_settings where key = 'tir_teacher_key'`)
	if code, _ := httpJSON(t, ts, "GET", "/api/game/results?key=whatever", nil); code != http.StatusServiceUnavailable {
		t.Fatalf("без ключа в базе: %d, want 503", code)
	}

	if _, err := env.Pool.Exec(ctx, `insert into system_settings (key, value) values ('tir_teacher_key', 'secret-42')
		on conflict (key) do update set value = excluded.value`); err != nil {
		t.Fatalf("ключ: %v", err)
	}
	if code, _ := httpJSON(t, ts, "GET", "/api/game/results?key=wrong", nil); code != http.StatusUnauthorized {
		t.Fatalf("чужой ключ: %d, want 401", code)
	}
	if code, _ := httpJSON(t, ts, "GET", "/api/game/results", nil); code != http.StatusUnauthorized {
		t.Fatalf("пустой ключ: %d, want 401", code)
	}

	code, resp := httpJSON(t, ts, "GET", "/api/game/results?key=secret-42&group=ри-410002", nil)
	if code != http.StatusOK {
		t.Fatalf("свой ключ: %d body=%+v", code, resp)
	}
	results, _ := resp["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("результатов %d, want 1: %+v", len(results), resp)
	}
	row, _ := results[0].(map[string]any)
	if row["family"] != "Сидоров" || row["group"] != "РИ-410002" {
		t.Fatalf("строка: %+v", row)
	}
	answers, _ := row["answers"].([]any)
	if len(answers) != 12 {
		t.Fatalf("ответы не вернулись целиком: пришло %d из 12", len(answers))
	}
	// A breakdown of every answer is what the cabinet exists for: the task, the
	// statement number and the outcome are visible.
	if a, _ := answers[0].(map[string]any); a["task"] != "1.1" || a["id"] == nil {
		t.Fatalf("ответ без задания или номера: %+v", answers[0])
	}

	if code, resp := httpJSON(t, ts, "GET", "/api/game/results?key=secret-42&group=РИ-999999", nil); code != http.StatusOK {
		t.Fatalf("фильтр по чужой группе: %d %+v", code, resp)
	} else if rs, _ := resp["results"].([]any); len(rs) != 0 {
		t.Fatalf("фильтр по группе не работает: %d строк", len(rs))
	}
}
