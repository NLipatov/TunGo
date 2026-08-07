package session

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeReaper tracks ReapIdle calls for testing the loop.
type fakeReaper struct {
	mu      sync.Mutex
	calls   []time.Duration
	results []int
	callIdx int
}

func (f *fakeReaper) ReapIdle(timeout time.Duration) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, timeout)
	result := 0
	if f.callIdx < len(f.results) {
		result = f.results[f.callIdx]
	}
	f.callIdx++
	return result
}

func (f *fakeReaper) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestRunIdleReaperLoop_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reaper := &fakeReaper{}

	done := make(chan struct{})
	go func() {
		RunIdleReaperLoop(ctx, reaper.ReapIdle, 30*time.Second, 10*time.Millisecond)
		close(done)
	}()

	// Let the reaper tick at least once
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reaper loop did not stop after context cancel")
	}
}

func TestRunIdleReaperLoop_CallsReapIdleWithCorrectTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reaper := &fakeReaper{}
	timeout := 42 * time.Second

	go RunIdleReaperLoop(ctx, reaper.ReapIdle, timeout, 10*time.Millisecond)

	// Wait for at least one tick
	time.Sleep(30 * time.Millisecond)
	cancel()

	if reaper.callCount() == 0 {
		t.Fatal("expected at least one ReapIdle call")
	}

	reaper.mu.Lock()
	defer reaper.mu.Unlock()
	for i, got := range reaper.calls {
		if got != timeout {
			t.Fatalf("call %d: expected timeout %v, got %v", i, timeout, got)
		}
	}
}

func TestRunIdleReaperLoop_MultipleTicksAccumulate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reaper := &fakeReaper{}

	go RunIdleReaperLoop(ctx, reaper.ReapIdle, 30*time.Second, 5*time.Millisecond)

	time.Sleep(40 * time.Millisecond)
	cancel()

	if reaper.callCount() < 3 {
		t.Fatalf("expected at least 3 ticks, got %d", reaper.callCount())
	}
}
