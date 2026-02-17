package session

import (
	"context"
	"strings"
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

// fakeLogger captures log output.
type fakeLogger struct {
	mu   sync.Mutex
	logs []string
}

func (l *fakeLogger) Printf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, format)
}

func (l *fakeLogger) containsSubstring(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.logs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestRunIdleReaperLoop_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reaper := &fakeReaper{}
	logger := &fakeLogger{}

	done := make(chan struct{})
	go func() {
		RunIdleReaperLoop(ctx, reaper, 30*time.Second, 10*time.Millisecond, logger)
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
	logger := &fakeLogger{}
	timeout := 42 * time.Second

	go RunIdleReaperLoop(ctx, reaper, timeout, 10*time.Millisecond, logger)

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

func TestRunIdleReaperLoop_LogsWhenSessionsReaped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reaper := &fakeReaper{results: []int{3, 0, 2}}
	logger := &fakeLogger{}

	go RunIdleReaperLoop(ctx, reaper, 30*time.Second, 10*time.Millisecond, logger)

	// Wait for enough ticks
	time.Sleep(50 * time.Millisecond)
	cancel()

	if !logger.containsSubstring("reaped %d idle session(s)") {
		t.Fatalf("expected reap log message, got %v", logger.logs)
	}
}

func TestRunIdleReaperLoop_DoesNotLogWhenNothingReaped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reaper := &fakeReaper{results: []int{0, 0, 0}}
	logger := &fakeLogger{}

	go RunIdleReaperLoop(ctx, reaper, 30*time.Second, 10*time.Millisecond, logger)

	time.Sleep(50 * time.Millisecond)
	cancel()

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.logs) != 0 {
		t.Fatalf("expected no logs when nothing reaped, got %v", logger.logs)
	}
}

func TestRunIdleReaperLoop_MultipleTicksAccumulate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reaper := &fakeReaper{}
	logger := &fakeLogger{}

	go RunIdleReaperLoop(ctx, reaper, 30*time.Second, 5*time.Millisecond, logger)

	time.Sleep(40 * time.Millisecond)
	cancel()

	if reaper.callCount() < 3 {
		t.Fatalf("expected at least 3 ticks, got %d", reaper.callCount())
	}
}
