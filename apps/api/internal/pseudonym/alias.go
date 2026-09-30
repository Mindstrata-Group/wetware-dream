package pseudonym

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kinds and alias labels; must match apps/anonymizer/anonymizer/kinds.py.
var labelsRU = map[string]string{
	"PERSON": "ЛИЦО", "PHONE": "ТЕЛЕФОН", "EMAIL": "ПОЧТА", "URL": "ССЫЛКА", "HANDLE": "НИК",
	"ADDRESS": "АДРЕС", "PLACE": "МЕСТО", "ORG": "ОРГ", "DATE": "ДАТА", "DOCUMENT": "ДОКУМЕНТ",
	"CARD": "КАРТА", "ACCOUNT": "СЧЁТ",
}

var labelsEN = map[string]string{
	"PERSON": "PERSON", "PHONE": "PHONE", "EMAIL": "EMAIL", "URL": "LINK", "HANDLE": "HANDLE",
	"ADDRESS": "ADDRESS", "PLACE": "PLACE", "ORG": "ORG", "DATE": "DATE", "DOCUMENT": "DOCUMENT",
	"CARD": "CARD", "ACCOUNT": "ACCOUNT",
}

// kindOfLabel maps both spellings back to the kind.
var kindOfLabel = func() map[string]string {
	out := map[string]string{"СЧЕТ": "ACCOUNT"}
	for kind, l := range labelsRU {
		out[l] = kind
	}
	for kind, l := range labelsEN {
		out[l] = kind
	}
	return out
}()

// ValidKind reports whether the detector's kind is one this package knows.
func ValidKind(kind string) bool {
	_, ok := labelsRU[kind]
	return ok
}

// Alias renders the Russian label ("LITSO_3" in Cyrillic) or "PERSON_3".
func Alias(kind, lang string, number int) string {
	table := labelsRU
	if lang == "en" {
		table = labelsEN
	}
	return table[kind] + "_" + strconv.Itoa(number)
}

var labelAlternation = func() string {
	labels := make([]string, 0, len(kindOfLabel))
	for l := range kindOfLabel {
		labels = append(labels, regexp.QuoteMeta(l))
	}
	// longest first so a long label is never read as a shorter one
	sort.Slice(labels, func(i, j int) bool { return len(labels[i]) > len(labels[j]) })
	return strings.Join(labels, "|")
}()

// aliasRE finds aliases, with an optional ending glued by the model
// (up to three Cyrillic letters, e.g. a dative "-u"). Go regexp has no
// lookbehind, so word boundaries are checked by hand.
var aliasRE = regexp.MustCompile(`(` + labelAlternation + `)_(\d{1,5})([а-яё]{1,3})?`)

type aliasMatch struct {
	start, end int // byte offsets
	kind       string
	number     int
	glued      string
}

func isWordRune(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
		r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'ё' || r == 'Ё'
}

func findAliases(text string) []aliasMatch {
	var out []aliasMatch
	for _, loc := range aliasRE.FindAllStringSubmatchIndex(text, -1) {
		start, end := loc[0], loc[1]
		if prev, _ := utf8.DecodeLastRuneInString(text[:start]); start > 0 && isWordRune(prev) {
			continue
		}
		if next, _ := utf8.DecodeRuneInString(text[end:]); end < len(text) && isWordRune(next) {
			continue
		}
		n, _ := strconv.Atoi(text[loc[4]:loc[5]])
		m := aliasMatch{start: start, end: end, kind: kindOfLabel[text[loc[2]:loc[3]]], number: n}
		if loc[6] >= 0 {
			m.glued = text[loc[6]:loc[7]]
		}
		out = append(out, m)
	}
	return out
}

// restoreNominative is the fallback when the Python service is down: aliases
// get the stored value as is (nominative), unknown aliases are marked.
func restoreNominative(text string, entries map[entryKey]Entry) string {
	matches := findAliases(text)
	if len(matches) == 0 {
		return text
	}
	var b strings.Builder
	cursor := 0
	for _, m := range matches {
		b.WriteString(text[cursor:m.start])
		if e, ok := entries[entryKey{m.kind, m.number}]; ok {
			b.WriteString(e.Canon)
			if m.kind != "PERSON" && m.kind != "PLACE" {
				b.WriteString(m.glued)
			}
		} else {
			b.WriteString("⟨" + text[m.start:m.end-len(m.glued)] + "⟩" + m.glued)
		}
		cursor = m.end
	}
	b.WriteString(text[cursor:])
	return b.String()
}
