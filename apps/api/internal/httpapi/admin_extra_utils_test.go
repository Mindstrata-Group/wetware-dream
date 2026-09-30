package httpapi

import "testing"

func TestUniquePositiveIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []int64
		want []int64
	}{
		{"empty", nil, []int64{}},
		{"already unique", []int64{1, 2, 3}, []int64{1, 2, 3}},
		{"removes duplicates", []int64{1, 2, 2, 3, 1}, []int64{1, 2, 3}},
		{"removes zeros and negatives", []int64{0, -1, -100, 5}, []int64{5}},
		{"all invalid", []int64{0, -1, -2}, []int64{}},
		{"preserves order", []int64{9, 3, 7, 3, 9}, []int64{9, 3, 7}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := uniquePositiveIDs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("uniquePositiveIDs(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %d, want %d", i, got[i], tc.want[i])
				}
			}
		})
	}
}
