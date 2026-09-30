package pseudonym

import (
	"context"
	"strings"
	"unicode/utf8"
)

// StreamRestorer restores aliases in a streamed answer (SSE chunks).
//
// A chunk boundary can cut an alias anywhere: "PER" + "SON_1", "PERSON_" +
// "1", even after the digits when the model glues a Cyrillic case ending on
// (the Russian label plus "-u"). The restorer holds
// back the shortest tail that could still grow into an alias and releases
// everything before it. Text without aliases passes through untouched and
// without a call to the service.
type StreamRestorer struct {
	svc     *Service
	ctx     context.Context
	entries []Entry
	pending string // raw model text not yet released
	emitted string // raw model text already released (tail kept for case context)
}

// NewStreamRestorer loads the user's vault once for the whole answer.
func (s *Service) NewStreamRestorer(ctx context.Context, userID int64, ref SessionRef) *StreamRestorer {
	entries, _ := s.Vault.Load(ctx, userID)
	return &StreamRestorer{svc: s, ctx: ctx, entries: entries}
}

// Write accepts the next chunk and returns the text that is safe to show.
func (r *StreamRestorer) Write(chunk string) string {
	r.pending += chunk
	cut := holdBack(r.pending)
	ready := r.pending[:cut]
	r.pending = r.pending[cut:]
	return r.release(ready)
}

// Flush releases whatever is left at the end of the answer.
func (r *StreamRestorer) Flush() string {
	ready := r.pending
	r.pending = ""
	return r.release(ready)
}

func (r *StreamRestorer) release(raw string) string {
	if raw == "" {
		return ""
	}
	out := r.svc.restoreWith(r.ctx, r.entries, r.emitted, raw)
	r.emitted = lastRunes(r.emitted+raw, 200)
	return out
}

func lastRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[len(runes)-n:])
}

// holdBack returns the byte index where the possibly unfinished alias at the
// end of text starts (len(text) if there is none).
//
// Held back: a partial label ("PERS" or the start of a Russian label), a
// label with the underscore,
// a complete alias with or without a glued ending that touches the end of
// the text — more digits or ending letters may still arrive. A trailing
// word is held only if it is a prefix of a label; plain words are released.
func holdBack(text string) int {
	// start of the trailing run of word characters
	start := len(text)
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !isWordRune(r) {
			break
		}
		start -= size
	}
	tail := text[start:]
	if tail == "" {
		return len(text)
	}
	if couldBeAlias(tail) {
		return start
	}
	return len(text)
}

func couldBeAlias(tail string) bool {
	label, rest, hasUnderscore := strings.Cut(tail, "_")
	if !hasUnderscore {
		for l := range kindOfLabel {
			if strings.HasPrefix(l, tail) {
				return true
			}
		}
		return false
	}
	if _, ok := kindOfLabel[label]; !ok {
		return false
	}
	// rest: digits, then up to three lowercase Cyrillic letters
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i > 5 {
		return false
	}
	letters := rest[i:]
	if i == 0 {
		return letters == ""
	}
	n := 0
	for _, r := range letters {
		if !(r >= 'а' && r <= 'я' || r == 'ё') {
			return false
		}
		n++
	}
	return n <= 3
}
