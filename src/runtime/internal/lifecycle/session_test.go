package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestSessionWait_ReturnsRuntimeError(t *testing.T) {
	session := New(func() {})
	want := errors.New("boom")
	session.Finish(want)

	if got := session.Wait(); !errors.Is(got, want) {
		t.Fatalf("expected runtime error, got %v", got)
	}
}

func TestSessionWait_ReturnsCancellation(t *testing.T) {
	session := New(func() {})
	session.Finish(context.Canceled)

	if got := session.Wait(); !errors.Is(got, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", got)
	}
}

func TestSessionAccessorsAndStop(t *testing.T) {
	stopCalls := 0
	session := New(func() { stopCalls++ })

	session.Stop()
	session.Stop()
	if stopCalls != 1 {
		t.Fatalf("expected Stop to invoke callback once, got %d", stopCalls)
	}
}

func TestSessionMarkReady_IsIdempotent(t *testing.T) {
	session := New(func() {})
	session.MarkReady()
	session.MarkReady()

	if err := session.WaitForReady(context.Background()); err != nil {
		t.Fatalf("expected ready session, got %v", err)
	}
}

func TestSessionWaitForReady_ReturnsTerminalErrorBeforeReady(t *testing.T) {
	session := New(func() {})
	want := errors.New("startup failed")
	session.Finish(want)

	if err := session.WaitForReady(context.Background()); !errors.Is(err, want) {
		t.Fatalf("expected startup error, got %v", err)
	}
}

func TestSessionWaitForReady_ReturnsStoppedBeforeReady(t *testing.T) {
	session := New(func() {})
	session.Finish(nil)

	if err := session.WaitForReady(context.Background()); !errors.Is(err, ErrStoppedBeforeReady) {
		t.Fatalf("expected ErrStoppedBeforeReady, got %v", err)
	}
}

func TestSessionWaitForReady_ReturnsContextError(t *testing.T) {
	session := New(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := session.WaitForReady(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
