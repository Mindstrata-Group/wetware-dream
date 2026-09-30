package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Unit tests for pure functions (no DB, no HTTP).
// They cover chat_message_helpers.go + admin_export_collect.go + admin_export_promo_modes.go.

// ============================================================================
// chat_message_helpers.go
// ============================================================================

func TestNormalizeResponseMode(t *testing.T) {
	cases := map[string]string{
		"live":   "live",
		"LIVE":   "live",
		" Live ": "live",
		"test":   "test",
		"":       "test",
		"foo":    "test",
		"prod":   "test",
	}
	for in, want := range cases {
		if got := normalizeResponseMode(in); got != want {
			t.Errorf("normalizeResponseMode(%q) = %q want %q", in, got, want)
		}
	}
}

func TestSlugModeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello World", "hello_world"},
		{"abc-def/ghi\\jkl", "abc_def_ghi_jkl"},
		{"Имя Режима", "имя_режима"},
		{"  Trim Me  ", "trim_me"},
		{"!@#$%^&*()", "mode"},
		{"", "mode"},
		{strings.Repeat("a", 50), strings.Repeat("a", 24)},
		{"___multiple___underscores___", "multiple___underscores"},
		{"ёлка", "ёлка"},
	}
	for _, c := range cases {
		if got := slugModeName(c.in); got != c.want {
			t.Errorf("slugModeName(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestBuildTestSummary(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "ack"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "done"},
	}
	got := buildTestSummary("MyMode", 42, msgs)

	if !strings.Contains(got, "Режим: MyMode") {
		t.Errorf("missing mode name: %s", got)
	}
	if !strings.Contains(got, "Dialog ID: 42") {
		t.Errorf("missing dialog id: %s", got)
	}
	if !strings.Contains(got, "Сообщений в сессии: 4") {
		t.Errorf("missing msg count: %s", got)
	}
	if !strings.Contains(got, `"second"`) {
		t.Errorf("did not pick last user message: %s", got)
	}
}

func TestBuildTestSummary_NoUserMessages(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "assistant", Content: "only assistant"},
	}
	got := buildTestSummary("ModeA", 7, msgs)
	if !strings.Contains(got, `""`) {
		t.Errorf("expected empty last-user quotes when no user messages, got %s", got)
	}
}

func TestBuildTestSummary_EmptyMessages(t *testing.T) {
	got := buildTestSummary("ModeB", 1, nil)
	if !strings.Contains(got, "Сообщений в сессии: 0") {
		t.Errorf("expected 0 count, got %s", got)
	}
}

// ============================================================================
// admin_export_collect.go
// ============================================================================

func TestAdminExportRoleFilter(t *testing.T) {
	cases := map[string]string{
		"assistant": "assistant",
		"user":      "user",
		"all":       "all",
		"  user  ":  "user",
		"":          "all",
		"unknown":   "all",
		"system":    "all",
		"USER":      "all", // case-sensitive — uppercase falls through to default
	}
	for in, want := range cases {
		if got := adminExportRoleFilter(in); got != want {
			t.Errorf("adminExportRoleFilter(%q) = %q want %q", in, got, want)
		}
	}
}

func TestAdminExportRoleWhere(t *testing.T) {
	cases := map[string]string{
		"assistant": "dm.role = 'assistant'",
		"user":      "dm.role = 'user'",
		"all":       "dm.role in ('user','assistant')",
		"unknown":   "dm.role in ('user','assistant')",
		"":          "dm.role in ('user','assistant')",
	}
	for in, want := range cases {
		if got := adminExportRoleWhere(in); got != want {
			t.Errorf("adminExportRoleWhere(%q) = %q want %q", in, got, want)
		}
	}
}

func TestAdminExportLimit(t *testing.T) {
	cases := map[string]int64{
		"100":       100,
		"  500  ":   500,
		"0":         0,
		"-5":        0,
		"":          0,
		"abc":       0,
		"1000000":   1000000,
		"1000001":   0, // exactly at the boundary that triggers cap
		"999999999": 0,
	}
	for in, want := range cases {
		if got := adminExportLimit(in); got != want {
			t.Errorf("adminExportLimit(%q) = %d want %d", in, got, want)
		}
	}
}

func TestAdminExportLimitInt(t *testing.T) {
	if got := adminExportLimitInt(0); got != 0 {
		t.Errorf("0: got %d", got)
	}
	if got := adminExportLimitInt(-1); got != 0 {
		t.Errorf("-1: got %d", got)
	}
	if got := adminExportLimitInt(1000000); got != 1000000 {
		t.Errorf("1M: got %d want 1000000", got)
	}
	if got := adminExportLimitInt(1000001); got != 0 {
		t.Errorf("1M+1: got %d want 0", got)
	}
}

func TestParseAdminExportTime_RFC3339(t *testing.T) {
	got, ok := parseAdminExportTime("2026-03-15T10:30:00Z", false)
	if !ok {
		t.Fatalf("ok=false for valid RFC3339")
	}
	if got.UTC().Format(time.RFC3339) != "2026-03-15T10:30:00Z" {
		t.Errorf("got %s", got)
	}
}

func TestParseAdminExportTime_DateOnly_StartOfDay(t *testing.T) {
	got, ok := parseAdminExportTime("2026-03-15", false)
	if !ok {
		t.Fatalf("ok=false")
	}
	if got.Hour() != 0 || got.Minute() != 0 {
		t.Errorf("expected 00:00, got %s", got)
	}
}

func TestParseAdminExportTime_DateOnly_EndOfDay(t *testing.T) {
	got, ok := parseAdminExportTime("2026-03-15", true)
	if !ok {
		t.Fatalf("ok=false")
	}
	// Should be 23:59:59.999... — almost next-day.
	next := got.Add(time.Nanosecond)
	if next.Day() != 16 {
		t.Errorf("endOfDay didn't land near midnight: got %s", got)
	}
}

func TestParseAdminExportTime_Invalid(t *testing.T) {
	for _, in := range []string{"", "not-a-date", "2026/03/15", "15-03-2026"} {
		if _, ok := parseAdminExportTime(in, false); ok {
			t.Errorf("expected ok=false for %q", in)
		}
	}
}

func TestParseAdminExportTime_TrimSpaces(t *testing.T) {
	if _, ok := parseAdminExportTime("  2026-03-15  ", false); !ok {
		t.Errorf("trim spaces should accept date")
	}
}

func TestApproxTokenCount(t *testing.T) {
	if got := approxTokenCount(""); got != 0 {
		t.Errorf("empty: got %d", got)
	}
	if got := approxTokenCount("abcd"); got != 1 {
		t.Errorf("4 chars: got %d want 1", got)
	}
	if got := approxTokenCount("abcdefgh"); got != 2 {
		t.Errorf("8 chars: got %d want 2", got)
	}
	if got := approxTokenCount("ab"); got != 1 {
		t.Errorf("2 chars: got %d want 1 (round up)", got)
	}
	// 16 runes including non-ASCII (1 rune each, not bytes)
	if got := approxTokenCount("приветприветприве"); got != 5 {
		t.Errorf("17 cyrillic runes: got %d want 5", got)
	}
}

func TestNullablePositive(t *testing.T) {
	if got := nullablePositive(0); got != nil {
		t.Errorf("0 should be nil, got %v", got)
	}
	if got := nullablePositive(-5); got != nil {
		t.Errorf("negative should be nil, got %v", got)
	}
	if got := nullablePositive(42); got != int64(42) {
		t.Errorf("42 should be int64(42), got %v", got)
	}
}

// ============================================================================
// admin_export_promo_modes.go
// ============================================================================

func TestAdminPromoModeIDsFromIndex(t *testing.T) {
	allModes := []int64{1, 2, 3, 4, 5}
	tariffModes := map[int64][]int64{
		100: {1, 2},
		200: {3, 4},
	}
	groupModes := map[int64][]int64{
		10: {1, 2, 3},
		20: {4, 5},
	}

	cases := []struct {
		name       string
		grantsType string
		targetID   int64
		want       []int64
	}{
		{"all", "all", 0, []int64{1, 2, 3, 4, 5}},
		{"all_modes", "all_modes", 0, []int64{1, 2, 3, 4, 5}},
		{"all_modes_access", "all_modes_access", 0, []int64{1, 2, 3, 4, 5}},
		{"single mode by target", "mode", 2, []int64{2}},
		{"single mode alias modes", "modes", 3, []int64{3}},
		{"single mode alias single_mode", "single_mode", 4, []int64{4}},
		{"single mode alias access_to_mode", "access_to_mode", 5, []int64{5}},
		{"single mode without target", "mode", 0, []int64{}},
		{"tariff lookup", "tariff", 100, []int64{1, 2}},
		{"tariff alias tariffs", "tariffs", 200, []int64{3, 4}},
		{"tariff alias plan", "plan", 100, []int64{1, 2}},
		{"tariff missing", "tariff", 999, nil},
		{"group lookup", "group", 10, []int64{1, 2, 3}},
		{"group alias tariff_group", "tariff_group", 20, []int64{4, 5}},
		{"group alias tariff_groups", "tariff_groups", 10, []int64{1, 2, 3}},
		{"group missing", "group", 999, nil},
		{"unknown grants_type with target → fallback", "weird", 42, []int64{42}},
		{"unknown grants_type without target → empty", "weird", 0, []int64{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := adminPromoModeIDsFromIndex(c.grantsType, c.targetID, allModes, tariffModes, groupModes)
			if !equalInt64Slice(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestAdminPromoModeIDsFromIndex_DoesNotMutateInput(t *testing.T) {
	allModes := []int64{1, 2, 3}
	originalLen := len(allModes)

	got := adminPromoModeIDsFromIndex("all", 0, allModes, nil, nil)
	got[0] = 999

	if allModes[0] != 1 {
		t.Errorf("input mutated: allModes[0] = %d", allModes[0])
	}
	if len(allModes) != originalLen {
		t.Errorf("input length changed")
	}
}

func equalInt64Slice(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ============================================================================
// access_promocode_helpers.go — reorderFirstMode
// ============================================================================

func modeItems(ids ...int64) []accessModeItem {
	out := make([]accessModeItem, len(ids))
	for i, id := range ids {
		out[i] = accessModeItem{ModeID: id}
	}
	return out
}

func modeIDs(items []accessModeItem) []int64 {
	ids := make([]int64, len(items))
	for i, m := range items {
		ids[i] = m.ModeID
	}
	return ids
}

func TestReorderFirstMode_BringsTargetToFront(t *testing.T) {
	got := modeIDs(reorderFirstMode(modeItems(1, 2, 3, 4), 3))
	want := []int64{3, 1, 2, 4}
	if !equalInt64Slice(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestReorderFirstMode_AlreadyFirst(t *testing.T) {
	got := modeIDs(reorderFirstMode(modeItems(1, 2, 3), 1))
	want := []int64{1, 2, 3}
	if !equalInt64Slice(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestReorderFirstMode_NotFound(t *testing.T) {
	got := modeIDs(reorderFirstMode(modeItems(1, 2, 3), 99))
	want := []int64{1, 2, 3}
	if !equalInt64Slice(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestReorderFirstMode_SingleItem(t *testing.T) {
	got := modeIDs(reorderFirstMode(modeItems(5), 5))
	want := []int64{5}
	if !equalInt64Slice(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestReorderFirstMode_Empty(t *testing.T) {
	got := reorderFirstMode(nil, 1)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestReorderFirstMode_LastItem(t *testing.T) {
	got := modeIDs(reorderFirstMode(modeItems(10, 20, 30), 30))
	want := []int64{30, 10, 20}
	if !equalInt64Slice(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// ============================================================================
// chat_attachments.go
// ============================================================================

func TestSanitizeAttachmentFilename(t *testing.T) {
	cases := map[string]string{
		"report.txt":              "report.txt",
		"../../../etc/passwd":     "passwd",
		"../../../etc/passwd.txt": "passwd.txt",
		"":                        "attachment.txt",
		".":                       "attachment.txt",
		"  spaced.md  ":           "spaced.md",
		"file\x00with\x01control": "filewithcontrol",
	}
	for in, want := range cases {
		if got := sanitizeAttachmentFilename(in); got != want {
			t.Errorf("sanitizeAttachmentFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeAttachmentFilename_TruncatesLongNamesKeepingExtension(t *testing.T) {
	long := strings.Repeat("a", 300) + ".docx"
	got := sanitizeAttachmentFilename(long)
	if len(got) > 180 {
		t.Fatalf("truncated filename still too long: %d chars", len(got))
	}
	if !strings.HasSuffix(got, ".docx") {
		t.Fatalf("truncation must preserve the extension, got %q", got)
	}
}

func TestAllowedAttachmentExtension(t *testing.T) {
	allowed := map[string]bool{"txt": true, "md": true, "doc": true, "docx": true, "exe": false, "": false, "pdf": false}
	for ext, want := range allowed {
		if got := allowedAttachmentExtension(ext); got != want {
			t.Errorf("allowedAttachmentExtension(%q) = %v, want %v", ext, got, want)
		}
	}
}

// TestUniquePositiveInt64s_StableOrderByFirstOccurrence: non-empty, with
// duplicates and negative values, in shuffled order: this is the branch that
// really runs indexOfInt64 inside sort.SliceStable (with length 0-1 Go does not
// call the comparator at all).
func TestUniquePositiveInt64s_StableOrderByFirstOccurrence(t *testing.T) {
	in := []int64{30, -5, 10, 30, 20, 0, 10}
	got := uniquePositiveInt64s(in)
	want := []int64{30, 10, 20}
	if !equalInt64Slice(got, want) {
		t.Errorf("got %v want %v (order must match first occurrence in input)", got, want)
	}
}

func TestUniquePositiveInt64s_Empty(t *testing.T) {
	if got := uniquePositiveInt64s(nil); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

// ============================================================================
// handlers.go: noopModeHistoryStore (History: MODE_HISTORY_DEPLOY_KEY is not set
// in env: every method must silently no-op, not panic/fail).
// ============================================================================

func TestNoopModeHistoryStore_AllMethodsAreSilentNoOp(t *testing.T) {
	var store noopModeHistoryStore
	if store.configured() {
		t.Fatalf("noop store must report configured()=false")
	}
	sha, err := store.commitSnapshot(context.Background(), 1, modeSnapshot{}, "a@b.c", "a@b.c", "msg")
	if sha != "" || err != nil {
		t.Fatalf("commitSnapshot = (%q, %v), want (\"\", nil)", sha, err)
	}
	entries, err := store.history(context.Background(), 1, 50)
	if entries != nil || err != nil {
		t.Fatalf("history = (%v, %v), want (nil, nil)", entries, err)
	}
	_, err = store.snapshotAt(context.Background(), 1, "deadbeef")
	if err != errModeHistoryNotConfigured {
		t.Fatalf("snapshotAt err = %v, want errModeHistoryNotConfigured", err)
	}
}
