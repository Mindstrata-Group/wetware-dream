package pseudonym

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFindAliasesBoundaries(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		"ЛИЦО_1":                    1,
		"с ЛИЦО_12, PERSON_3.":      2,
		"ЛИЦО_1у и ТЕЛЕФОН_2":       2,
		"XЛИЦО_1":                   0,
		"ЛИЦО_1X":                   0,
		"ЛИЦО_":                     0,
		"СЧЕТ_4 и ДОКУМЕНТ_2":       2,
		"прочиталЛИЦО_1 слитно":     0,
		"(ЛИЦО_1)":                  1,
		"ЛИЦО_1ушла — не окончание": 0,
	}
	for text, want := range cases {
		if got := len(findAliases(text)); got != want {
			t.Errorf("%q: got %d aliases, want %d", text, got, want)
		}
	}
}

func TestHoldBack(t *testing.T) {
	t.Parallel()
	cases := map[string]string{ // text -> held tail
		"Поговорите с ":        "",
		"Поговорите с Л":       "Л",
		"Поговорите с ЛИЦ":     "ЛИЦ",
		"Поговорите с ЛИЦО_":   "ЛИЦО_",
		"Поговорите с ЛИЦО_1":  "ЛИЦО_1",
		"Поговорите с ЛИЦО_1у": "ЛИЦО_1у",
		"Поговорите с ЛИЦО_1 ": "",
		"Поговорите спокойно":  "",
		"Call PERS":            "PERS",
		"Call Peter":           "",
		"номер ТЕЛЕФОН_12":     "ТЕЛЕФОН_12",
		"ЛИЦО_1234567":         "",
	}
	for text, want := range cases {
		cut := holdBack(text)
		if got := text[cut:]; got != want {
			t.Errorf("%q: held %q, want %q", text, got, want)
		}
	}
}

// Splitting the answer at every possible point must give the same result as
// restoring it whole.
func TestStreamRestorerAnySplit(t *testing.T) {
	t.Parallel()
	svc, _, vault := newService(nil)
	vault.rows[1] = []Entry{
		{Kind: "PERSON", Number: 1, Canon: "Люда", Key: "люда"},
		{Kind: "PHONE", Number: 1, Canon: "89123456789", Key: "9123456789"},
	}
	answer := "Поговорите с ЛИЦО_1 спокойно. Номер ТЕЛЕФОН_1 сохраните, а ЛИЦО_2 — кто это?"
	whole := svc.Restore(context.Background(), 1, "v1:ru", answer)
	runes := []rune(answer)
	for i := 0; i <= len(runes); i++ {
		for j := i; j <= len(runes); j += 3 {
			r := svc.NewStreamRestorer(context.Background(), 1, "v1:ru")
			var b strings.Builder
			b.WriteString(r.Write(string(runes[:i])))
			b.WriteString(r.Write(string(runes[i:j])))
			b.WriteString(r.Write(string(runes[j:])))
			b.WriteString(r.Flush())
			if b.String() != whole {
				t.Fatalf("split at %d/%d:\n got %q\nwant %q", i, j, b.String(), whole)
			}
		}
	}
}

func TestStreamRestorerPassesContextForCases(t *testing.T) {
	t.Parallel()
	svc, det, vault := newService(nil)
	vault.rows[1] = []Entry{{Kind: "PERSON", Number: 1, Canon: "Люда", Key: "люда"}}
	var seen []string
	det.restoreFn = func(text, contextText string, entries []Entry) (RestoreResult, error) {
		seen = append(seen, contextText)
		return RestoreResult{Text: strings.ReplaceAll(text, "ЛИЦО_1", "Людой")}, nil
	}
	r := svc.NewStreamRestorer(context.Background(), 1, "v1:ru")
	out := r.Write("Поговорите с ") + r.Write("ЛИЦО_1") + r.Write(" спокойно.") + r.Flush()
	if out != "Поговорите с Людой спокойно." {
		t.Fatalf("%q", out)
	}
	if len(seen) != 1 || !strings.HasSuffix(seen[0], "Поговорите с ") {
		t.Fatalf("the service must get the text before the alias as context: %q", seen)
	}
}

func TestLastRunes(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("я", 300)
	if got := lastRunes(s, 200); utf8.RuneCountInString(got) != 200 || !utf8.ValidString(got) {
		t.Fatal("lastRunes must cut on rune boundaries")
	}
}
