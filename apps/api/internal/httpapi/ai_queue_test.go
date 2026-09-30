package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"
)

func resetAIProviderQueueForTest(t *testing.T) {
	t.Helper()
	select {
	case <-aiProviderQueue.mu:
		aiProviderQueue.active = 0
		aiProviderQueue.nextStart = time.Time{}
		aiProviderQueue.mu <- struct{}{}
	case <-time.After(time.Second):
		t.Fatal("timed out acquiring aiProviderQueue mutex")
	}
}

func aiProviderQueueActiveForTest(t *testing.T) int {
	t.Helper()
	select {
	case <-aiProviderQueue.mu:
		active := aiProviderQueue.active
		aiProviderQueue.mu <- struct{}{}
		return active
	case <-time.After(time.Second):
		t.Fatal("timed out acquiring aiProviderQueue mutex")
	}
	return -1
}

func TestAIQueue_IntervalEnforcedBetweenSlots(t *testing.T) {
	resetAIProviderQueueForTest(t)
	settings := aiQueueSettings{Enabled: true, Interval: 80 * time.Millisecond, Timeout: time.Second, Concurrency: 1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	firstStart := time.Now()
	if err := acquireAIProviderSlot(ctx, settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	releaseAIProviderSlot()

	if err := acquireAIProviderSlot(ctx, settings); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	defer releaseAIProviderSlot()

	if elapsed := time.Since(firstStart); elapsed < 70*time.Millisecond {
		t.Fatalf("second slot started too early after %s", elapsed)
	}
}

func TestAIQueue_ConcurrencyBlocksUntilRelease(t *testing.T) {
	resetAIProviderQueueForTest(t)
	settings := aiQueueSettings{Enabled: true, Interval: 0, Timeout: time.Second, Concurrency: 1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := acquireAIProviderSlot(ctx, settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	acquired := make(chan error, 1)
	go func() {
		acquired <- acquireAIProviderSlot(ctx, settings)
	}()

	select {
	case err := <-acquired:
		t.Fatalf("second acquire completed before release: %v", err)
	case <-time.After(40 * time.Millisecond):
	}

	releaseAIProviderSlot()
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("second acquire after release: %v", err)
		}
		releaseAIProviderSlot()
	case <-time.After(time.Second):
		t.Fatal("second acquire did not complete after release")
	}
}

func TestAIQueue_AcquireHonorsContextTimeout(t *testing.T) {
	resetAIProviderQueueForTest(t)
	settings := aiQueueSettings{Enabled: true, Interval: 0, Timeout: time.Second, Concurrency: 1}
	if err := acquireAIProviderSlot(context.Background(), settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer releaseAIProviderSlot()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := acquireAIProviderSlot(ctx, settings)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquire err=%v want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestAIQueue_ReleaseRestoresCapacity(t *testing.T) {
	resetAIProviderQueueForTest(t)
	settings := aiQueueSettings{Enabled: true, Interval: 0, Timeout: time.Second, Concurrency: 1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := acquireAIProviderSlot(ctx, settings); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	releaseAIProviderSlot()
	if active := aiProviderQueueActiveForTest(t); active != 0 {
		t.Fatalf("active after release=%d want 0", active)
	}
	if err := acquireAIProviderSlot(ctx, settings); err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	releaseAIProviderSlot()
}

func TestAIQueue_ExtraReleaseDoesNotMakeActiveNegative(t *testing.T) {
	resetAIProviderQueueForTest(t)
	releaseAIProviderSlot()
	if active := aiProviderQueueActiveForTest(t); active < 0 {
		t.Fatalf("active after extra release=%d want non-negative", active)
	}
}
