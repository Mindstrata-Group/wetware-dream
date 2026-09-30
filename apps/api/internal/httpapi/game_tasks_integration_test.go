//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func setTeacherKey(t testing.TB, env *testsupport.Env, key string) {
	t.Helper()
	if _, err := env.Pool.Exec(context.Background(),
		`insert into system_settings (key, value) values ('tir_teacher_key', $1)
		 on conflict (key) do update set value = excluded.value`, key); err != nil {
		t.Fatalf("ключ преподавателя: %v", err)
	}
}

func taskItem(task string, id int, answer any, text string) map[string]any {
	return map[string]any{
		"task": task, "id": id, "t": text, "answer": answer,
		"lvl": 1, "src": "it", "why": "объяснение", "marks": []int{1, 2}, "doc": false,
	}
}

// TestGameTasks_SaveArchiveRestore: editing the bank from the teacher's cabinet.
// There is deliberately no deletion: submitted reports reference the record
// number, so we check archiving and restoring from the archive.
func TestGameTasks_SaveArchiveRestore(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "k-tasks-1")

	body := map[string]any{"action": "save", "item": taskItem("1.1", 9001, 3, "Проверочное утверждение про принцип")}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/tasks?key=k-tasks-1", body); code != http.StatusOK {
		t.Fatalf("сохранение: %d %+v", code, resp)
	}

	// Answers are served only with the key: without it the answer field is stripped
	// (that was the hole through which the whole bank was downloaded).
	code, resp := httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1&key=k-tasks-1", nil)
	if code != http.StatusOK {
		t.Fatalf("чтение: %d %+v", code, resp)
	}
	items, _ := resp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("ожидали одну запись, получили %d: %+v", len(items), resp)
	}
	got, _ := items[0].(map[string]any)
	if got["id"] != float64(9001) || got["answer"] != float64(3) {
		t.Fatalf("запись вернулась искажённой: %+v", got)
	}

	// Editing the same record does not create a duplicate, it updates the text.
	body = map[string]any{"action": "save", "item": taskItem("1.1", 9001, 4, "Утверждение переписано и стало законом")}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/tasks?key=k-tasks-1", body); code != http.StatusOK {
		t.Fatalf("повторное сохранение: %d %+v", code, resp)
	}
	_, resp = httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1&key=k-tasks-1", nil)
	items, _ = resp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("после правки записей стало %d, want 1", len(items))
	}
	if got, _ := items[0].(map[string]any); got["answer"] != float64(4) {
		t.Fatalf("правка не применилась: %+v", got)
	}

	// Archiving removes the record from what the game serves, but does not lose it.
	arch := map[string]any{"action": "archive", "item": map[string]any{"task": "1.1", "id": 9001}}
	if code, resp := httpJSON(t, ts, "POST", "/api/game/tasks?key=k-tasks-1", arch); code != http.StatusOK {
		t.Fatalf("архивация: %d %+v", code, resp)
	}
	_, resp = httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1&key=k-tasks-1", nil)
	if items, _ := resp["items"].([]any); len(items) != 0 {
		t.Fatalf("архивная запись осталась в выдаче для игры: %+v", items)
	}
	_, resp = httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1&archived=1&key=k-tasks-1", nil)
	if items, _ := resp["items"].([]any); len(items) != 1 {
		t.Fatalf("архивная запись не видна преподавателю: %+v", resp)
	}

	rest := map[string]any{"action": "restore", "item": map[string]any{"task": "1.1", "id": 9001}}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/tasks?key=k-tasks-1", rest); code != http.StatusOK {
		t.Fatalf("возврат из архива: %d", code)
	}
	_, resp = httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1&key=k-tasks-1", nil)
	if items, _ := resp["items"].([]any); len(items) != 1 {
		t.Fatalf("после возврата запись не появилась: %+v", resp)
	}
}

// TestGameTasks_RejectsWrongAnswerType: the main check of this file.
// In 1.2 the answer is boolean, in 1.3 a string, in 1.1 and 1.4 a number 1–5. A
// record with the wrong answer type would never match the student's choice: the
// task would become impossible, and only a student in the seminar would notice.
func TestGameTasks_RejectsWrongAnswerType(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "k-tasks-2")

	cases := []struct {
		name   string
		task   string
		answer any
		want   int
	}{
		{"в 1.2 число вместо булева", "1.2", 3, http.StatusBadRequest},
		{"в 1.1 строка вместо числа", "1.1", "fi", http.StatusBadRequest},
		{"в 1.1 число вне диапазона", "1.1", 9, http.StatusBadRequest},
		{"в 1.3 неизвестный вид", "1.3", "xxx", http.StatusBadRequest},
		{"в 1.4 дробное", "1.4", 2.5, http.StatusBadRequest},
		{"в 1.2 булево — годится", "1.2", true, http.StatusOK},
		{"в 1.3 известный вид — годится", "1.3", "rnd", http.StatusOK},
	}
	for i, c := range cases {
		body := map[string]any{"action": "save", "item": taskItem(c.task, 9100+i, c.answer, "Утверждение для проверки типа ответа")}
		if code, resp := httpJSON(t, ts, "POST", "/api/game/tasks?key=k-tasks-2", body); code != c.want {
			t.Fatalf("%s: код %d, ожидали %d (%+v)", c.name, code, c.want, resp)
		}
	}
}

// TestGameTasks_KeyRequiredForWrites: the bank can be read without the key (it is
// text the student sees anyway), but edited only with the key.
func TestGameTasks_KeyRequiredForWrites(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "k-tasks-3")

	body := map[string]any{"action": "save", "item": taskItem("1.1", 9200, 1, "Утверждение без ключа не сохраняется")}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/tasks", body); code != http.StatusUnauthorized {
		t.Fatalf("без ключа: %d, want 401", code)
	}
	if code, _ := httpJSON(t, ts, "POST", "/api/game/tasks?key=wrong", body); code != http.StatusUnauthorized {
		t.Fatalf("с чужим ключом: %d, want 401", code)
	}
	// The bank used to be readable without the key: that is how it was downloaded along with the answers.
	if code, _ := httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1", nil); code != http.StatusUnauthorized {
		t.Fatalf("чтение без ключа: %d, want 401", code)
	}
	if code, _ := httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1&archived=1", nil); code != http.StatusUnauthorized {
		t.Fatalf("архив без ключа: %d, want 401", code)
	}
	if code, _ := httpJSON(t, ts, "GET", "/api/game/tasks?task=1.1&key=k-tasks-3", nil); code != http.StatusOK {
		t.Fatalf("чтение с ключом: %d, want 200", code)
	}
}

// TestGameTasks_Import: loading the bank from files is repeatable: a second import
// of the same records does not create duplicates, it updates them.
func TestGameTasks_Import(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	setTeacherKey(t, env, "k-tasks-4")

	items := []map[string]any{
		taskItem("1.2", 9301, true, "Первое утверждение импорта"),
		taskItem("1.2", 9302, false, "Второе утверждение импорта"),
		taskItem("1.2", 9303, 5, "Третье — с негодным ответом, должно отсеяться"),
	}
	body := map[string]any{"action": "import", "items": items}
	code, resp := httpJSON(t, ts, "POST", "/api/game/tasks?key=k-tasks-4", body)
	if code != http.StatusOK {
		t.Fatalf("импорт: %d %+v", code, resp)
	}
	if resp["saved"] != float64(2) || resp["received"] != float64(3) {
		t.Fatalf("импорт посчитан неверно: %+v", resp)
	}

	if code, resp = httpJSON(t, ts, "POST", "/api/game/tasks?key=k-tasks-4", body); code != http.StatusOK {
		t.Fatalf("повторный импорт: %d %+v", code, resp)
	}
	_, resp = httpJSON(t, ts, "GET", "/api/game/tasks?task=1.2&key=k-tasks-4", nil)
	if items, _ := resp["items"].([]any); len(items) != 2 {
		t.Fatalf("после повторного импорта записей %d, want 2 — появились дубли", len(items))
	}
}
