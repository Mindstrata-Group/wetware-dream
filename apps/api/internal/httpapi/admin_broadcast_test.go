package httpapi

import (
	"testing"
	"time"
)

func ptr(t time.Time) *time.Time { return &t }

func TestPromocodeIsActive(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	cases := []struct {
		name      string
		from, to  *time.Time
		maxUses   int
		usedCount int
		want      bool
	}{
		{"no limits always active", nil, nil, 0, 0, true},
		{"not yet started", ptr(future), nil, 0, 0, false},
		{"already expired", nil, ptr(past), 0, 0, false},
		{"within window", ptr(past), ptr(future), 0, 0, true},
		{"uses exhausted", nil, nil, 10, 10, false},
		{"uses not exhausted", nil, nil, 10, 9, true},
		{"zero maxUses means unlimited", nil, nil, 0, 999, true},
		{"expired and uses exhausted", nil, ptr(past), 5, 5, false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := promocodeIsActive(tc.from, tc.to, tc.maxUses, tc.usedCount)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseAdminIDList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		values []string
		single string
		want   []int64
	}{
		{"empty", nil, "", []int64{}},
		{"single param", nil, "42", []int64{42}},
		{"comma-separated", []string{"1,2,3"}, "", []int64{1, 2, 3}},
		{"multiple values", []string{"1", "2"}, "3", []int64{1, 2, 3}},
		{"deduplication", []string{"5,5"}, "5", []int64{5}},
		{"negatives ignored", []string{"-1,0"}, "2", []int64{2}},
		{"spaces trimmed", []string{" 7 , 8 "}, "", []int64{7, 8}},
		{"invalid strings ignored", []string{"abc", "1"}, "", []int64{1}},
		{"mixed valid/invalid", []string{"3,x,5"}, "", []int64{3, 5}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseAdminIDList(tc.values, tc.single)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %d, want %d", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTemporaryAdminDefaultNote(t *testing.T) {
	t.Parallel()

	got := temporaryAdminDefaultNote("")
	if got == "" {
		t.Error("empty purpose should return default note, got empty string")
	}
	custom := "Custom note"
	if got := temporaryAdminDefaultNote(custom); got != custom {
		t.Errorf("got %q, want %q", got, custom)
	}
	// whitespace-only treated as empty → returns default
	got2 := temporaryAdminDefaultNote("   ")
	if got2 == "" || got2 == "   " {
		t.Error("whitespace purpose should return default note")
	}
}
