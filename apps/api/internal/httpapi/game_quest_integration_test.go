//go:build integration

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// Practice 2 quests after the rework: tasks with partial credit.
// The whole path is checked: bank import, serving without answers, points for
// closeness and for the option weight, the "task shown" mark.

const questKey = "k-quest"

func questRateItem(variant, step, answer int, slot string, bonus bool) map[string]any {
	payload := map[string]any{
		"slot": slot, "doc": "Заявка в Фонд Дун-Дук", "author": "м.н.с. Пук-Ли",
		"body":  fmt.Sprintf("Фрагмент %d: проект относится к прикладным исследованиям, гипотеза проверяется на открытом датасете.", step),
		"scale": []int{3, 10},
	}
	if bonus {
		payload["bonus"] = true
	}
	return map[string]any{
		"task": "2.1", "id": variant*100 + step, "variant": variant, "step": step,
		"t": fmt.Sprintf("Черновик %d", step), "kind": "rate", "payload": payload,
		"answer": answer, "why": "разбор", "lvl": 2, "src": "kejs",
	}
}

func questChoiceItem(variant, step int, slot string) map[string]any {
	return map[string]any{
		"task": "2.2", "id": 2000 + variant*100 + step, "variant": variant, "step": step,
		"t": "Статья: воспроизводимость", "kind": "choice",
		"payload": map[string]any{
			"slot": slot, "body": "Рецензент спрашивает, можно ли повторить результат по тексту статьи.",
			"options": []map[string]string{
				{"id": "a", "text": "Можно: код и данные открыты, гиперпараметры в приложении"},
				{"id": "b", "text": "Частично: код есть, но важные детали только в репозитории"},
				{"id": "c", "text": "Нельзя: это статья, а не код"},
			},
		},
		"answer": map[string]any{"credit": map[string]any{"a": 0.5, "b": 1, "c": 0}},
		"why":    "разбор", "lvl": 2, "src": "kejs",
	}
}

func importQuest(t testing.TB, ts *TestServer, items []map[string]any) map[string]any {
	t.Helper()
	code, resp := httpJSON(t, ts, "POST", "/api/game/tasks?key="+questKey, map[string]any{"action": "import", "items": items})
	if code != http.StatusOK {
		t.Fatalf("импорт банка: %d %+v", code, resp)
	}
	return resp
}

func startQuest(t testing.TB, ts *TestServer, task string, variant int, family string) (string, []map[string]any) {
	t.Helper()
	code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "start", "task": task, "variant": variant,
		"family": family, "name": "Тест", "group": "РИ-2", "mate": "Напарник Н.",
	})
	if code != http.StatusOK {
		t.Fatalf("старт квеста: %d %+v", code, resp)
	}
	sid, _ := resp["sessionId"].(string)
	raw, _ := resp["items"].([]any)
	items := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		it, _ := r.(map[string]any)
		items = append(items, it)
	}
	return sid, items
}

func questAnswer(t testing.TB, env *testsupport.Env, ts *TestServer, sid string, itemID int, value any) map[string]any {
	t.Helper()
	thinkABit(t, env, sid)
	code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "answer", "sessionId": sid, "itemId": itemID, "value": value,
	})
	if code != http.StatusOK {
		t.Fatalf("ответ на %d: %d %+v", itemID, code, resp)
	}
	return resp
}

// TestQuest_ImportRejectsBrokenTasks: bad tasks are rejected with a reason,
// not dropped silently.
func TestQuest_ImportRejectsBrokenTasks(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, questKey)

	outOfScale := questRateItem(1, 2, 12, "w1", false)
	twoFull := questChoiceItem(1, 1, "repro")
	twoFull["answer"] = map[string]any{"credit": map[string]any{"a": 1, "b": 1, "c": 0}}
	resp := importQuest(t, ts, []map[string]any{questRateItem(1, 1, 7, "w1", false), outOfScale, twoFull})
	if resp["saved"] != float64(1) {
		t.Fatalf("ожидали одно принятое задание: %+v", resp)
	}
	rejected, _ := resp["rejected"].([]any)
	if len(rejected) != 2 {
		t.Fatalf("ожидали две причины отказа: %+v", resp)
	}
	joined := fmt.Sprint(rejected)
	for _, why := range []string{"вне шкалы", "ровно у одного"} {
		if !strings.Contains(joined, why) {
			t.Fatalf("в отказах нет причины %q: %s", why, joined)
		}
	}
}

// TestQuest_RateCreditAndBonus: points for closeness to the expert, the extra
// point separately, and nothing leaks out: no reference answer and no
// explanation before the answer.
func TestQuest_RateCreditAndBonus(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, questKey)
	importQuest(t, ts, []map[string]any{
		questRateItem(3, 1, 7, "w1", false),
		questRateItem(3, 2, 5, "w1", false),
		questRateItem(3, 3, 9, "w4", true),
	})

	sid, items := startQuest(t, ts, "2.1", 3, "Оценщиков")
	if len(items) != 3 {
		t.Fatalf("выдано %d заданий, want 3", len(items))
	}
	for _, it := range items {
		if it["kind"] != "rate" {
			t.Fatalf("вид задания потерялся: %+v", it)
		}
		p, _ := it["payload"].(map[string]any)
		if p["body"] == nil || p["scale"] == nil || p["slot"] == nil {
			t.Fatalf("нет текста, шкалы или слота: %+v", it)
		}
		for _, leak := range []string{"answer", "why", "truth", "credit"} {
			if _, found := it[leak]; found {
				t.Fatalf("в задании до ответа поле %q: %+v", leak, it)
			}
		}
	}

	// Reference values 7, 5, 9. We answer 8 (off by 1), 5 (exact), 6 (off by 3).
	want := []float64{0.75, 1, 0.15}
	for i, v := range []int{8, 5, 6} {
		resp := questAnswer(t, env, ts, sid, int(items[i]["id"].(float64)), v)
		if resp["credit"] != want[i] {
			t.Fatalf("задание %d: балл %v, want %v (%+v)", i, resp["credit"], want[i], resp)
		}
		if resp["why"] == nil || resp["truth"] == nil {
			t.Fatalf("после ответа нет эталона и разбора: %+v", resp)
		}
	}

	code, fin := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid})
	if code != http.StatusOK {
		t.Fatalf("закрытие: %d %+v", code, fin)
	}
	if fin["credit"] != 1.75 || fin["creditTotal"] != float64(2) || fin["bonusCredit"] != 0.15 || fin["bonusTotal"] != float64(1) {
		t.Fatalf("итог сессии: %+v, want основная 1.75 из 2, дополнительная 0.15 из 1", fin)
	}
}

// TestQuest_ChoicePartialCredit: a defensible but incomplete answer earns part
// of the credit and does not hit the score like a mistake.
func TestQuest_ChoicePartialCredit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, questKey)
	importQuest(t, ts, []map[string]any{questChoiceItem(4, 1, "repro"), questChoiceItem(4, 2, "repro")})

	sid, items := startQuest(t, ts, "2.2", 4, "Рецензентов")
	first := questAnswer(t, env, ts, sid, int(items[0]["id"].(float64)), "a")
	if first["credit"] != 0.5 || first["correct"] != false {
		t.Fatalf("защитимый вариант: %+v, want балл 0.5 и не «верно»", first)
	}
	if d, _ := first["delta"].(float64); d <= 0 {
		t.Fatalf("частичный балл оштрафован как ошибка: delta=%v", first["delta"])
	}
	second := questAnswer(t, env, ts, sid, int(items[1]["id"].(float64)), "b")
	if second["credit"] != float64(1) || second["correct"] != true {
		t.Fatalf("лучший вариант: %+v", second)
	}
}

// TestQuest_ShowStartsTheClock: between tasks the student plays the quest.
// Without the "shown" mark an honest answer after a long scene counted as late.
func TestQuest_ShowStartsTheClock(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, questKey)
	importQuest(t, ts, []map[string]any{questRateItem(5, 1, 7, "w1", false), questRateItem(5, 2, 7, "w2", false)})
	sid, items := startQuest(t, ts, "2.1", 5, "Долгоиграев")
	id1 := int(items[0]["id"].(float64))
	id2 := int(items[1]["id"].(float64))

	// Only the expected task can be marked.
	if code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "show", "sessionId": sid, "itemId": id2}); code != http.StatusBadRequest {
		t.Fatalf("отметка чужого задания: %d %+v, want 400", code, resp)
	}

	// Twenty minutes of quest since the start.
	if _, err := env.Pool.Exec(context.Background(),
		`update game_sessions set last_seen_at = now() - interval '20 minutes' where id = $1::uuid`, sid); err != nil {
		t.Fatal(err)
	}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "show", "sessionId": sid, "itemId": id1}); code != http.StatusOK {
		t.Fatalf("отметка показа: %d %+v", code, resp)
	}
	resp := questAnswer(t, env, ts, sid, id1, 7)
	if resp["timedOut"] == true || resp["credit"] != float64(1) {
		t.Fatalf("ответ после долгой сцены ушёл в опоздание: %+v", resp)
	}

	// Without the mark, late is still late.
	if _, err := env.Pool.Exec(context.Background(),
		`update game_sessions set last_seen_at = now() - interval '20 minutes' where id = $1::uuid`, sid); err != nil {
		t.Fatal(err)
	}
	code, late := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "answer", "sessionId": sid, "itemId": id2, "value": 7})
	if code != http.StatusOK || late["timedOut"] != true || late["credit"] != float64(0) {
		t.Fatalf("ответ через 20 минут без показа засчитан: %d %+v", code, late)
	}
}

// TestQuest_ShowOnlyForQuests: in the practice 1 shooting range time is part of
// the game, and it must not be reset by a mark.
func TestQuest_ShowOnlyForQuests(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	seedTasks(t, env, "1.1", 3)
	sid, items := startSession(t, ts, "1.1", "Тиров")
	it, _ := items[0].(map[string]any)
	code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "show", "sessionId": sid, "itemId": int(it["id"].(float64))})
	if code != http.StatusBadRequest {
		t.Fatalf("отметка показа в тире: %d %+v, want 400", code, resp)
	}
}

// TestQuest_ArchiveWorksForPractice2: the archive used to always look for the
// record in the practice 1 bank, so a 2.x task could not be removed.
func TestQuest_ArchiveWorksForPractice2(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, questKey)
	importQuest(t, ts, []map[string]any{questRateItem(6, 1, 7, "w1", false)})
	arch := map[string]any{"action": "archive", "item": map[string]any{"task": "2.1", "id": 601}}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/tasks?key="+questKey, arch); code != http.StatusOK {
		t.Fatalf("архивация задания 2.1: %d %+v", code, resp)
	}
}

// TestQuest_RefreshDoesNotCloseLiveQuest: on entry the page restores progress,
// and the server closes abandoned attempts "after a minute of silence". In the
// quest there are quarter-hour stretches between exams with no answers at all:
// reloading the page in the middle of "Glavred" would close a live game, and
// every next answer would get "session already closed".
func TestQuest_RefreshDoesNotCloseLiveQuest(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, questKey)
	importQuest(t, ts, []map[string]any{questRateItem(7, 1, 7, "w1", false), questRateItem(7, 2, 6, "w2", false)})
	sid, items := startQuest(t, ts, "2.1", 7, "Обновляев")
	questAnswer(t, env, ts, sid, int(items[0]["id"].(float64)), 7)

	// Fifteen minutes of quest, then F5: the page calls restore.
	if _, err := env.Pool.Exec(context.Background(),
		`update game_sessions set last_seen_at = now() - interval '15 minutes' where id = $1::uuid`, sid); err != nil {
		t.Fatal(err)
	}
	code, rest := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "restore", "family": "Обновляев", "name": "Тест", "group": "РИ-2",
	})
	if code != http.StatusOK {
		t.Fatalf("восстановление: %d %+v", code, rest)
	}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "show", "sessionId": sid, "itemId": int(items[1]["id"].(float64))}); code != http.StatusOK {
		t.Fatalf("после обновления страницы квест закрыт: %d %+v", code, resp)
	}
	resp := questAnswer(t, env, ts, sid, int(items[1]["id"].(float64)), 7)
	if resp["credit"] != 0.75 {
		t.Fatalf("ответ после обновления: %+v", resp)
	}

	// A closed quest is restored with partial credit, not "right/wrong".
	if code, fin := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid}); code != http.StatusOK {
		t.Fatalf("закрытие: %d %+v", code, fin)
	}
	_, rest = httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "restore", "family": "Обновляев", "name": "Тест", "group": "РИ-2",
	})
	best, _ := rest["best"].([]any)
	if len(best) != 1 {
		t.Fatalf("восстановлено %d дел, want 1: %+v", len(best), rest)
	}
	b, _ := best[0].(map[string]any)
	if b["credit"] != 1.75 || b["creditTotal"] != float64(2) {
		t.Fatalf("восстановленное дело без частичного балла: %+v", b)
	}
}

// TestQuest_ClientLog: the game log is written into the session in order, cuts
// batches and foreign sessions, and is accepted even after the case is closed.
func TestQuest_ClientLog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, questKey)
	importQuest(t, ts, []map[string]any{questRateItem(8, 1, 7, "w1", false), questRateItem(8, 2, 6, "w1", false)})
	sid, _ := startQuest(t, ts, "2.1", 8, "Журналов")

	send := func(events []map[string]any) (int, map[string]any) {
		return httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "log", "sessionId": sid, "events": events})
	}
	if code, resp := send([]map[string]any{{"ev": "step", "loc": 60}, {"ev": "step", "loc": 63, "game": "fail"}}); code != http.StatusOK || resp["kept"] != float64(2) {
		t.Fatalf("запись журнала: %d %+v", code, resp)
	}
	big := make([]map[string]any, 51)
	for i := range big {
		big[i] = map[string]any{"ev": "x"}
	}
	if code, _ := send(big); code != http.StatusBadRequest {
		t.Fatalf("пачка из 51 события принята: %d", code)
	}
	if code, _ := send([]map[string]any{{"ev": strings.Repeat("я", 500)}}); code != http.StatusBadRequest {
		t.Fatalf("событие больше 800 байт принято: %d", code)
	}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/session", map[string]any{
		"action": "log", "sessionId": "00000000-0000-4000-8000-00000000abcd", "events": []map[string]any{{"ev": "x"}},
	}); code != http.StatusNotFound {
		t.Fatalf("журнал чужой сессии: %d, want 404", code)
	}
	httpJSON(t, ts, "POST", "/api/game/session", map[string]any{"action": "finish", "sessionId": sid})
	if code, resp := send([]map[string]any{{"ev": "end"}}); code != http.StatusOK || resp["kept"] != float64(3) {
		t.Fatalf("журнал после закрытия: %d %+v", code, resp)
	}
	var first string
	if err := env.Pool.QueryRow(context.Background(), `select client_log->0->>'ev' || ':' || (client_log->1->>'loc') from game_sessions where id = $1::uuid`, sid).Scan(&first); err != nil || first != "step:63" {
		t.Fatalf("порядок журнала: %q %v", first, err)
	}
}
