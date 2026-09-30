package httpapi

import (
	"testing"
	"time"
)

// The tests below work with the shared global buffer clientLogEntries and run in
// parallel with integration tests that also write to it.
// So no "clear the buffer and check len": each test tags its entries with a
// unique UserID and checks only their presence/absence.
// (Regression 2026-06-09: tests that swapped the global slice were flaky in CI
// when parallel tests posted to client-log between setup and the check.)

const (
	clientLogTestUserAppend = 4200001
	clientLogTestUserOld    = 4200002
	clientLogTestUserFresh  = 4200003
)

func clientLogFindByUser(entries []clientLogEntry, userID int64) *clientLogEntry {
	for i := range entries {
		if entries[i].UserID == userID {
			return &entries[i]
		}
	}
	return nil
}

// clientLogAppend adds an entry and clientLogGetAll returns it.
func TestClientLog_AppendAndGetAll(t *testing.T) {
	t.Parallel()

	entry := clientLogEntry{
		At:     time.Now(),
		UserID: clientLogTestUserAppend,
		Role:   "user",
		UA:     "TestAgent/1.0",
		Path:   "/chat",
		Events: []clientLogEvent{{Type: "onChange", DomLen: intPtr(5)}},
	}
	clientLogAppend(entry)

	got := clientLogFindByUser(clientLogGetAll(), clientLogTestUserAppend)
	if got == nil {
		t.Fatal("clientLogGetAll не вернул только что добавленную запись")
	}
	if len(got.Events) != 1 || got.Events[0].Type != "onChange" {
		t.Errorf("unexpected events: %+v", got.Events)
	}
}

// clientLogGetAll does not return entries older than 24 hours.
func TestClientLog_OldEntriesFiltered(t *testing.T) {
	t.Parallel()

	old := clientLogEntry{
		At:     time.Now().Add(-25 * time.Hour),
		UserID: clientLogTestUserOld,
		Role:   "guest",
		Events: []clientLogEvent{{Type: "blur"}},
	}
	// Insert directly, bypassing append (which would filter it itself).
	// At the start of the slice: entries are ordered by At, the filter only cuts from the head.
	clientLogMu.Lock()
	clientLogEntries = append([]clientLogEntry{old}, clientLogEntries...)
	clientLogMu.Unlock()

	if got := clientLogFindByUser(clientLogGetAll(), clientLogTestUserOld); got != nil {
		t.Fatalf("запись старше 24ч не отфильтрована: %+v", got)
	}
}

// Fresh entries are not filtered.
func TestClientLog_FreshEntriesKept(t *testing.T) {
	t.Parallel()

	fresh := clientLogEntry{
		At:     time.Now().Add(-1 * time.Hour),
		UserID: clientLogTestUserFresh,
		Role:   "admin",
		Events: []clientLogEvent{{Type: "input"}},
	}
	clientLogAppend(fresh)

	got := clientLogFindByUser(clientLogGetAll(), clientLogTestUserFresh)
	if got == nil {
		t.Fatal("свежая запись (-1ч) не должна фильтроваться")
	}
	if got.Role != "admin" {
		t.Errorf("Role=%q, want %q", got.Role, "admin")
	}
}

func intPtr(v int) *int { return &v }
