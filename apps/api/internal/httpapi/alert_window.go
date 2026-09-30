package httpapi

import (
	"sync"
	"time"
)

// alertWindow counts events over a sliding time window with NON-destructive
// reads.
//
// Why not `counter.Swap(0)`:
//
//   - A read with a side effect ("read -> reset") is correct for exactly
//     ONE observer. As soon as two poll the endpoint (Kuma + a human
//     with curl, the prod monitor + the staging monitor, two status tabs), they
//     eat each other's events, and some AI failures are seen by nobody.
//   - A separate flavour of the same problem is retries inside Kuma itself. With
//     `maxretries >= 1` the first (failed) request reset the counter, the retry saw
//     zero, the monitor silently went back to UP and no alert was sent at all.
//
// Here reads are idempotent: an event "lives" for exactly window time, and every
// observer inside the window sees the same picture. Anyone who wants deltas
// takes the monotonic total from the snapshot and subtracts it themselves.
//
// Storage is a ring of buckets of bucket seconds: O(1) memory for any error
// rate (unlike a list of timestamps), counting over the window is O(buckets).
type alertWindow struct {
	mu      sync.Mutex
	buckets []int64 // per-bucket event counters
	epochs  []int64 // the interval number the bucket currently belongs to
	bucket  time.Duration
	total   int64 // monotonic counter over the whole process lifetime
	lastAt  int64 // UnixNano of the last event, 0 if there were no events

	// now is replaced in tests to run the window without real pauses.
	now func() time.Time
}

const (
	// alertWindowBucket is the window granularity. 5 seconds is enough: alert
	// windows are measured in minutes.
	alertWindowBucket = 5 * time.Second
	// alertWindowMaxLookback is the maximum depth that can be requested at all.
	// It sets the ring size (memory); anything longer is not shown.
	alertWindowMaxLookback = 15 * time.Minute
)

// alertWindowSnapshot is a non-destructive snapshot of the window state.
type alertWindowSnapshot struct {
	// Count is how many events fell into the requested window.
	Count int64
	// Total is a monotonic counter since process start (never
	// reset; for observers that can compute deltas themselves).
	Total int64
	// Last is the time of the last event; zero if there were none.
	Last time.Time
}

func newAlertWindow(bucket, maxLookback time.Duration) *alertWindow {
	n := int(maxLookback / bucket)
	if n < 1 {
		n = 1
	}
	return &alertWindow{
		buckets: make([]int64, n),
		epochs:  make([]int64, n),
		bucket:  bucket,
		now:     time.Now,
	}
}

// epochAt returns the bucket number for a point in time.
func (w *alertWindow) epochAt(t time.Time) int64 {
	return t.UnixNano() / int64(w.bucket)
}

// record marks one event "now".
func (w *alertWindow) record() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	epoch := w.epochAt(now)
	idx := int(epoch % int64(len(w.buckets)))
	if idx < 0 {
		idx += len(w.buckets)
	}
	// Buckets are reused in a circle: if one holds a different interval,
	// that is stale data, so reset it.
	if w.epochs[idx] != epoch {
		w.epochs[idx] = epoch
		w.buckets[idx] = 0
	}
	w.buckets[idx]++
	w.total++
	w.lastAt = now.UnixNano()
}

// snapshot returns the window state WITHOUT changing anything: any number
// of observers get the same answer until the window expires on its own.
func (w *alertWindow) snapshot(window time.Duration) alertWindowSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()

	maxLookback := w.bucket * time.Duration(len(w.buckets))
	if window > maxLookback {
		window = maxLookback
	}
	if window < w.bucket {
		window = w.bucket
	}

	cur := w.epochAt(w.now())
	// The boundary is rounded up to a whole bucket: the window may include up to
	// bucket extra seconds. For an alert, over-counting is safer than under-counting:
	// an "extra" 503 wakes someone up, a missed one wakes nobody.
	oldest := cur - int64(window/w.bucket) + 1

	snap := alertWindowSnapshot{Total: w.total}
	for i, e := range w.epochs {
		if e >= oldest && e <= cur {
			snap.Count += w.buckets[i]
		}
	}
	if w.lastAt != 0 {
		snap.Last = time.Unix(0, w.lastAt)
	}
	return snap
}

// reset clears the window. Only tests need it: in prod the window expires on its own.
func (w *alertWindow) reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := range w.buckets {
		w.buckets[i] = 0
		w.epochs[i] = 0
	}
	w.total = 0
	w.lastAt = 0
}
