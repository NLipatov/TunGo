package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestSessionWait_ReturnsRuntimeError(t *testing.T) {
	session := New(context.Background(), closedReadyChForTest(), func() {})
	want := errors.New("boom")
	session.Finish(want)

	if got := session.Wait(); !errors.Is(got, want) {
		t.Fatalf("expected runtime error, got %v", got)
	}
}

func TestSessionWait_SuppressesCancellation(t *testing.T) {
	session := New(context.Background(), closedReadyChForTest(), func() {})
	session.Finish(context.Canceled)

	if got := session.Wait(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestSessionWait_SuppressesErrorsAfterContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := New(ctx, closedReadyChForTest(), cancel)
	cancel()
	session.Finish(errors.New("late error"))

	if got := session.Wait(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestSessionAccessorsAndStop(t *testing.T) {
	readyCh := closedReadyChForTest()
	stopped := false
	session := New(context.Background(), readyCh, func() { stopped = true })

	if session.Ready() != readyCh {
		t.Fatal("expected Ready to return configured channel")
	}
	if session.Done() == nil {
		t.Fatal("expected Done channel")
	}
	if session.Err() != nil {
		t.Fatalf("expected nil initial error, got %v", session.Err())
	}

	session.Stop()
	if !stopped {
		t.Fatal("expected Stop to call cancel function")
	}
}

func TestSessionNilContext(t *testing.T) {
	session := New(nil, closedReadyChForTest(), func() {})
	session.Finish(errors.New("runtime failed"))

	if got := session.Wait(); got == nil || got.Error() != "runtime failed" {
		t.Fatalf("expected runtime error with fallback context, got %v", got)
	}
}

func TestClosedReadyCh(t *testing.T) {
	select {
	case <-ClosedReadyCh():
	default:
		t.Fatal("expected ready channel to be closed")
	}
}

func closedReadyChForTest() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
