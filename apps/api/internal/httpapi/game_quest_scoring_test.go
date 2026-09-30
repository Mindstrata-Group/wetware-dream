package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

const questBody = "Проект относится к прикладным исследованиям: гипотеза проверяется на DFDC и собственных записях."

func ratePayload(scale ...int) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"slot": "w1", "body": questBody, "scale": scale})
	return b
}

func choicePayload(ids ...string) json.RawMessage {
	opts := []map[string]string{}
	for _, id := range ids {
		opts = append(opts, map[string]string{"id": id, "text": "Суждение " + id})
	}
	b, _ := json.Marshal(map[string]any{"slot": "repro", "body": questBody, "options": opts})
	return b
}

// TestQuestTaskCheck: a mistake in the bank silently makes a task impossible,
// so each of these tasks must be rejected on import.
func TestQuestTaskCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		kind    string
		payload json.RawMessage
		answer  string
		wantErr string
	}{
		{"оценка в шкале", gameKindRate, ratePayload(3, 10), `7`, ""},
		{"оценка вне шкалы", gameKindRate, ratePayload(3, 10), `11`, "вне шкалы"},
		{"оценка дробная", gameKindRate, ratePayload(3, 10), `7.5`, "целое"},
		{"шкала задом наперёд", gameKindRate, ratePayload(10, 3), `7`, "шкала"},
		{"выбор с одним полным баллом", gameKindChoice, choicePayload("a", "b", "c"), `{"credit":{"a":1,"b":0.5,"c":0}}`, ""},
		{"выбор с двумя полными", gameKindChoice, choicePayload("a", "b"), `{"credit":{"a":1,"b":1}}`, "ровно у одного"},
		{"выбор без полного", gameKindChoice, choicePayload("a", "b"), `{"credit":{"a":0.5,"b":0}}`, "ровно у одного"},
		{"вес чужого варианта", gameKindChoice, choicePayload("a", "b"), `{"credit":{"a":1,"z":0.5}}`, "несуществующего"},
		{"вес больше единицы", gameKindChoice, choicePayload("a", "b"), `{"credit":{"a":1,"b":1.5}}`, "от 0 до 1"},
		{"повтор номера варианта", gameKindChoice, choicePayload("a", "a"), `{"credit":{"a":1}}`, "повторными"},
		{"неизвестный вид", "binary", ratePayload(3, 10), `7`, "rate или choice"},
		{"без слота", gameKindRate, json.RawMessage(`{"body":"` + questBody + `","scale":[3,10]}`), `7`, "слота"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := gameQuestTaskCheck(tc.kind, tc.payload, json.RawMessage(tc.answer))
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("задание отбито зря: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("плохое задание пропущено, ждали ошибку %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("ошибка %q, ждали про %q", err, tc.wantErr)
			}
		})
	}
}

// TestQuestCredit_RateByDistance: points for closeness to the expert, as in the
// "literary test" of "Glavred". Off by one: almost full credit; off by four:
// zero. If the scale slips, students get "all or nothing" back, and nobody
// notices.
func TestQuestCredit_RateByDistance(t *testing.T) {
	t.Parallel()
	truth := json.RawMessage(`7`)
	for given, want := range map[float64]float64{7: 1, 8: 0.75, 6: 0.75, 5: 0.4, 9: 0.4, 4: 0.15, 10: 0.15, 3: 0} {
		if got := gameQuestCredit(gameKindRate, truth, given); got != want {
			t.Errorf("оценка %v при эталоне 7: балл %v, want %v", given, got, want)
		}
	}
	// An answer given as a string also counts: the request shape must not cost a point.
	if got := gameQuestCredit(gameKindRate, truth, "8"); got != 0.75 {
		t.Errorf("оценка строкой: %v, want 0.75", got)
	}
	if got := gameQuestCredit(gameKindRate, truth, map[string]any{"x": 1}); got != 0 {
		t.Errorf("мусор вместо оценки дал балл %v", got)
	}
}

func TestQuestCredit_ChoiceWeights(t *testing.T) {
	t.Parallel()
	truth := json.RawMessage(`{"credit":{"a":1,"b":0.5,"c":0}}`)
	for given, want := range map[string]float64{"a": 1, "b": 0.5, "c": 0, "zzz": 0} {
		if got := gameQuestCredit(gameKindChoice, truth, given); got != want {
			t.Errorf("вариант %q: %v, want %v", given, got, want)
		}
	}
}

// TestQuestPublic_NoAnswerLeak: the student gets only what is needed to answer.
// A draft note in the payload must not reach the browser.
func TestQuestPublic_NoAnswerLeak(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"slot":"w1","body":"` + questBody + `","scale":[3,10],"expert":7,"note":"эталон 7"}`)
	pub := gameQuestPublic(gameKindRate, raw)
	b, _ := json.Marshal(pub)
	for _, leak := range []string{"expert", "эталон", "note"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("в публичной части %q: %s", leak, b)
		}
	}
	if pub["body"] == nil || pub["scale"] == nil {
		t.Fatalf("в публичной части нет текста или шкалы: %s", b)
	}
}

// TestQuestCreditSums: the extra point (item 1.3.5 in task 2.1) is counted
// separately; otherwise it dilutes the main grade instead of adding to it.
func TestQuestCreditSums(t *testing.T) {
	t.Parallel()
	main, bonus := gameCreditSums([]map[string]any{
		{"credit": 1.0}, {"credit": 0.75}, {"credit": 0.4, "bonus": true},
		{"correct": true}, // old record without credit
	})
	if main.n != 3 || main.sum != 2.75 {
		t.Fatalf("основная часть: %+v, want 2.75 из 3", main)
	}
	if bonus.n != 1 || bonus.sum != 0.4 {
		t.Fatalf("дополнительный балл: %+v, want 0.4 из 1", bonus)
	}
}
