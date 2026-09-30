package httpapi

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAIQueue_AcquireReleaseHappyPath: one request takes a slot at once and
// returns without error; after release the next one also passes.
func TestAIQueue_AcquireReleaseHappyPath(t *testing.T) {
	resetAIProviderQueueForTest(t)

	settings := aiQueueSettings{
		Enabled:     true,
		Interval:    0, // no throttle for this test
		Timeout:     time.Second,
		Concurrency: 1,
	}

	start := time.Now()
	if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("first acquire took %v, expected near-instant", elapsed)
	}
	releaseAIProviderSlot()

	if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	releaseAIProviderSlot()
}

// TestAIQueue_ConcurrencyLimit: with concurrency=1 the second request must wait
// until the first releases. Without a timeout it would hang, so we use a small
// timeout.
func TestAIQueue_ConcurrencyLimit(t *testing.T) {
	resetAIProviderQueueForTest(t)

	settings := aiQueueSettings{
		Enabled:     true,
		Interval:    0,
		Timeout:     500 * time.Millisecond,
		Concurrency: 1,
	}

	// the first takes a slot and does not release it
	if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer releaseAIProviderSlot()

	// the second must block and fail on timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := acquireAIProviderSlot(ctx, settings)
	if err == nil {
		releaseAIProviderSlot() // in case it somehow got through
		t.Fatal("second acquire prošёл несмотря на concurrency=1 и истёкший ctx")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("err=%v, want DeadlineExceeded", err)
	}
}

// TestAIQueue_IntervalThrottle: with Interval=200ms the second request must
// wait ~200ms even if the first released its slot at once.
func TestAIQueue_IntervalThrottle(t *testing.T) {
	resetAIProviderQueueForTest(t)

	settings := aiQueueSettings{
		Enabled:     true,
		Interval:    200 * time.Millisecond,
		Timeout:     time.Second,
		Concurrency: 1,
	}

	if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	releaseAIProviderSlot()

	start := time.Now()
	if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	releaseAIProviderSlot()

	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond {
		t.Errorf("second acquire took %v, expected ≥150ms due to interval throttle", elapsed)
	}
}

// TestAIQueue_ContextCancel: if ctx is cancelled while waiting for a slot,
// ctx.Err() is returned (not a hang).
func TestAIQueue_ContextCancel(t *testing.T) {
	resetAIProviderQueueForTest(t)

	settings := aiQueueSettings{
		Enabled:     true,
		Interval:    0,
		Timeout:     time.Second,
		Concurrency: 1,
	}

	if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer releaseAIProviderSlot()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- acquireAIProviderSlot(ctx, settings)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("err=%v want Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("acquire не вернулся через 500ms после cancel — зависание")
	}
}

// TestAIQueue_ParallelAcquireRespectsLimit: 5 goroutines, concurrency=2,
// exactly 2 must hold a slot at the same time.
func TestAIQueue_ParallelAcquireRespectsLimit(t *testing.T) {
	resetAIProviderQueueForTest(t)

	settings := aiQueueSettings{
		Enabled:     true,
		Interval:    0,
		Timeout:     2 * time.Second,
		Concurrency: 2,
	}

	var holding atomic.Int32
	var maxHolding atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			n := holding.Add(1)
			// update max
			for {
				cur := maxHolding.Load()
				if n <= cur || maxHolding.CompareAndSwap(cur, n) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			holding.Add(-1)
			releaseAIProviderSlot()
		}()
	}
	wg.Wait()

	if got := maxHolding.Load(); got > 2 {
		t.Errorf("max concurrent holders=%d, want ≤2 (concurrency limit)", got)
	}
	if got := maxHolding.Load(); got < 1 {
		t.Errorf("max concurrent holders=%d, want ≥1 (sanity)", got)
	}
}
