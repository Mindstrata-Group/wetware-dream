//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestKeyBruteForceLocksOut: guessing the key must hit a ceiling. Five misses
// from an address, and after that it is refused even with the correct key;
// otherwise the ceiling is bypassed by guessing on the last attempt.
func TestKeyBruteForceLocksOut(t *testing.T) {
	// No t.Parallel: the limiter is process-wide, parallel tests would interfere.
	env := testsupport.NewEnv(t)
	// Protection is enabled on purpose: this is the only test that checks it.
	ts := NewTestServerWithHandler(t, env.Pool, Handler{})
	setTeacherKey(t, env, "верный-ключ-для-перебора")

	for i := 0; i < 6; i++ {
		httpJSON(t, ts, "GET", "/api/game/results?key=промах", nil)
	}
	// The correct key after brute force does not pass either.
	if code, _ := httpJSON(t, ts, "GET", "/api/game/results?key=верный-ключ-для-перебора", nil); code != http.StatusUnauthorized {
		t.Fatalf("после шести промахов верный ключ прошёл: %d", code)
	}

	var attempts int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from security_events where kind = 'bad_key'`).Scan(&attempts); err != nil {
		t.Fatalf("журнал: %v", err)
	}
	if attempts < 6 {
		t.Fatalf("в журнале %d попыток подбора, ожидали не меньше шести", attempts)
	}
}
