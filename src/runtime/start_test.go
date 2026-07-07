package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStart_InvalidMode(t *testing.T) {
	_, err := Start(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid runtime mode") {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}

func TestRunningSessionWait_ReturnsRuntimeError(t *testing.T) {
	session := newRunningSession(context.Background(), closedReadyChForTest(), func() {})
	want := errors.New("boom")
	session.finish(want)

	if got := session.Wait(); !errors.Is(got, want) {
		t.Fatalf("expected runtime error, got %v", got)
	}
}

func TestRunningSessionWait_SuppressesCancellation(t *testing.T) {
	session := newRunningSession(context.Background(), closedReadyChForTest(), func() {})
	session.finish(context.Canceled)

	if got := session.Wait(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestRunningSessionWait_SuppressesErrorsAfterContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := newRunningSession(ctx, closedReadyChForTest(), cancel)
	cancel()
	session.finish(errors.New("late error"))

	if got := session.Wait(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func closedReadyChForTest() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
