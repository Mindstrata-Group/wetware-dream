package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Partial credit in the practicum 2 quests.
//
// The first version of practicum 2 was a questionnaire with one right answer: "what type of
// research is this: FI, AI or R&D". Students guessed, and the teacher got a
// test that checks memory for definitions rather than judgement. Real
// peer review is not like that: three RSF reviewers agree on a grant application in only
// 16% of cases, and a good reviewer is distinguished not by "guessing right" but by
// their score being close to the score of knowledgeable people.
//
// So the quests have two new kinds of tasks:
//
//   rate   - score a fragment of an application on a scale (like the "literary test" in
//            "Glavred"). Credit is for closeness to the expert score: missing by
//            one is barely penalised, missing by four is zero.
//   choice - pick a reviewer's judgement. Each option has its own weight:
//            a defensible but incomplete position gets partial credit.
//
// The truth and the weights live only in the database. The student gets the payload: text,
// scale, options.

const (
	gameKindBinary = "binary"
	gameKindRate   = "rate"
	gameKindChoice = "choice"
)

// gameRateCredit is the credit for missing by delta scale divisions.
var gameRateCredit = []float64{1, 0.75, 0.4, 0.15}

type gameQuestPayload struct {
	Slot    string `json:"slot"`
	Doc     string `json:"doc"`
	Author  string `json:"author"`
	Body    string `json:"body"`
	Scale   []int  `json:"scale"`
	Bonus   bool   `json:"bonus"`
	Options []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"options"`
}

type gameChoiceAnswer struct {
	Credit map[string]float64 `json:"credit"`
}

// gameQuestTaskCheck validates a quest task as a whole: kind, public part and
// answer. A mistake in the bank costs more here than anywhere else: a task with a wrong scale
// or without a full-credit option becomes impossible, and it is noticed not by CI
// but by a student in class.
func gameQuestTaskCheck(kind string, payloadRaw, answerRaw json.RawMessage) error {
	var p gameQuestPayload
	if err := json.Unmarshal(payloadRaw, &p); err != nil {
		return fmt.Errorf("payload не разбирается")
	}
	if strings.TrimSpace(p.Slot) == "" || len(p.Slot) > 40 {
		return fmt.Errorf("в payload нет слота")
	}
	if n := len([]rune(p.Body)); n < 20 || n > 6000 {
		return fmt.Errorf("текст задания от 20 до 6000 знаков")
	}
	switch kind {
	case gameKindRate:
		if len(p.Scale) != 2 || p.Scale[0] >= p.Scale[1] || p.Scale[1]-p.Scale[0] > 20 {
			return fmt.Errorf("шкала задаётся парой [от, до]")
		}
		var v float64
		if err := json.Unmarshal(answerRaw, &v); err != nil || v != math.Trunc(v) {
			return fmt.Errorf("ответ оценки — целое число")
		}
		if int(v) < p.Scale[0] || int(v) > p.Scale[1] {
			return fmt.Errorf("ответ вне шкалы")
		}
		if len(p.Options) != 0 {
			return fmt.Errorf("у оценки нет вариантов")
		}
	case gameKindChoice:
		if len(p.Options) < 2 || len(p.Options) > 6 {
			return fmt.Errorf("вариантов от 2 до 6")
		}
		ids := map[string]bool{}
		for _, o := range p.Options {
			if o.ID == "" || len(o.ID) > 20 || ids[o.ID] || strings.TrimSpace(o.Text) == "" {
				return fmt.Errorf("варианты с пустыми или повторными номерами")
			}
			ids[o.ID] = true
		}
		var a gameChoiceAnswer
		if err := json.Unmarshal(answerRaw, &a); err != nil || len(a.Credit) == 0 {
			return fmt.Errorf("ответ выбора — веса вариантов")
		}
		full := 0
		for id, c := range a.Credit {
			if !ids[id] {
				return fmt.Errorf("вес у несуществующего варианта %q", id)
			}
			if c < 0 || c > 1 {
				return fmt.Errorf("вес варианта от 0 до 1")
			}
			if c == 1 {
				full++
			}
		}
		if full != 1 {
			return fmt.Errorf("полный балл должен быть ровно у одного варианта")
		}
	default:
		return fmt.Errorf("в квесте задания бывают rate или choice")
	}
	return nil
}

// gameQuestCredit is what an answer is worth. An unparseable answer is worth zero, not
// an error: a student must not get stuck on a task because of the request shape.
func gameQuestCredit(kind string, truthRaw json.RawMessage, given any) float64 {
	switch kind {
	case gameKindRate:
		var truth float64
		if err := json.Unmarshal(truthRaw, &truth); err != nil {
			return 0
		}
		g, ok := given.(float64)
		if !ok {
			if s, isStr := given.(string); isStr {
				if _, err := fmt.Sscanf(s, "%g", &g); err != nil {
					return 0
				}
			} else {
				return 0
			}
		}
		delta := int(math.Abs(math.Round(g) - truth))
		if delta < len(gameRateCredit) {
			return gameRateCredit[delta]
		}
		return 0
	case gameKindChoice:
		var a gameChoiceAnswer
		if err := json.Unmarshal(truthRaw, &a); err != nil {
			return 0
		}
		id, _ := given.(string)
		return a.Credit[id]
	}
	return 0
}

// gameQuestPublic is what goes to the student. It is rebuilt from the payload
// instead of being forwarded as is: an extra field accidentally put into the bank
// (for example a draft note "reference 7") must not leak to the browser.
func gameQuestPublic(kind string, payloadRaw json.RawMessage) map[string]any {
	var p gameQuestPayload
	if err := json.Unmarshal(payloadRaw, &p); err != nil {
		return nil
	}
	out := map[string]any{"slot": p.Slot, "body": p.Body}
	if p.Doc != "" {
		out["doc"] = p.Doc
	}
	if p.Author != "" {
		out["author"] = p.Author
	}
	if p.Bonus {
		out["bonus"] = true
	}
	switch kind {
	case gameKindRate:
		out["scale"] = p.Scale
	case gameKindChoice:
		opts := make([]map[string]string, 0, len(p.Options))
		for _, o := range p.Options {
			opts = append(opts, map[string]string{"id": o.ID, "text": o.Text})
		}
		out["options"] = opts
	}
	return out
}

// gameQuestBonus: the task counts towards the bonus point (item 1.3.5 in task 2.1).
func gameQuestBonus(payloadRaw json.RawMessage) bool {
	var p gameQuestPayload
	_ = json.Unmarshal(payloadRaw, &p)
	return p.Bonus
}

// gameCreditRound: credit is stored to two decimal places; in the report and the log
// "0.7500000001" only confuses people.
func gameCreditRound(c float64) float64 { return math.Round(c*100) / 100 }

type gameCreditSum struct {
	sum float64
	n   int
}

// gameCreditSums sums the session's credit separately for the main part and the
// bonus point. Old records have no credit field; there it is replaced by
// correct.
func gameCreditSums(given []map[string]any) (main, bonus gameCreditSum) {
	for _, a := range given {
		c, ok := a["credit"].(float64)
		if !ok {
			if a["correct"] == true {
				c = 1
			}
		}
		if a["bonus"] == true {
			bonus.sum += c
			bonus.n++
		} else {
			main.sum += c
			main.n++
		}
	}
	main.sum = gameCreditRound(main.sum)
	bonus.sum = gameCreditRound(bonus.sum)
	return main, bonus
}
