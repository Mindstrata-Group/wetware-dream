package pseudonym

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"
)

// fakeDetector marks every occurrence of the listed values, the way the
// Python service would, with code point offsets.
type fakeDetector struct {
	values    map[string]string // value -> kind
	residual  int
	detectErr error
	restoreFn func(text, contextText string, entries []Entry) (RestoreResult, error)
	mu        sync.Mutex
	calls     int
	lastKnown []Known
}

func (f *fakeDetector) Detect(_ context.Context, text, lang string, known []Known) (DetectResult, error) {
	f.mu.Lock()
	f.calls++
	f.lastKnown = known
	f.mu.Unlock()
	if f.detectErr != nil {
		return DetectResult{}, f.detectErr
	}
	var spans []Span
	for value, kind := range f.values {
		from := 0
		for {
			i := strings.Index(text[from:], value)
			if i < 0 {
				break
			}
			b := from + i
			start := utf8.RuneCountInString(text[:b])
			spans = append(spans, Span{Start: start, End: start + utf8.RuneCountInString(value), Kind: kind,
				Canon: value, Key: strings.ToLower(value)})
			from = b + len(value)
		}
	}
	if lang == "auto" {
		lang = "ru"
	}
	return DetectResult{Spans: spans, Residual: f.residual, Lang: lang}, nil
}

func (f *fakeDetector) Restore(_ context.Context, text, contextText string, entries []Entry) (RestoreResult, error) {
	if f.restoreFn != nil {
		return f.restoreFn(text, contextText, entries)
	}
	table := map[entryKey]Entry{}
	for _, e := range entries {
		table[entryKey{e.Kind, e.Number}] = e
	}
	return RestoreResult{Text: restoreNominative(text, table)}, nil
}

// memVault is an in-memory Vault with the same numbering rules as PGVault.
type memVault struct {
	mu   sync.Mutex
	rows map[int64][]Entry
	err  error
}

func newMemVault() *memVault { return &memVault{rows: map[int64][]Entry{}} }

func (v *memVault) Load(_ context.Context, userID int64) ([]Entry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.err != nil {
		return nil, v.err
	}
	return append([]Entry(nil), v.rows[userID]...), nil
}

func (v *memVault) Assign(_ context.Context, userID int64, kind, key, canon string) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.err != nil {
		return 0, v.err
	}
	max := 0
	for _, e := range v.rows[userID] {
		if e.Kind == kind && e.Key == key {
			return e.Number, nil
		}
		if e.Kind == kind && e.Number > max {
			max = e.Number
		}
	}
	v.rows[userID] = append(v.rows[userID], Entry{Kind: kind, Number: max + 1, Canon: canon, Key: key})
	return max + 1, nil
}

type staticProfiles map[int64]Profile

func (p staticProfiles) Profile(_ context.Context, userID int64) (Profile, error) {
	return p[userID], nil
}

var errBoom = errors.New("boom")
