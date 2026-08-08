package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type clientTestDeps struct {
	disposeCalls int
	disposeErr   error
}

func (d *clientTestDeps) runtime(
	runSession func(context.Context, func()) error,
) *clientRuntime {
	return &clientRuntime{
		runSession: runSession,
		disposeDevices: func() error {
			d.disposeCalls++
			return d.disposeErr
		},
	}
}

func TestClientRunAttemptSignalsReadyWhenSessionStarts(t *testing.T) {
	deps := &clientTestDeps{}
	started := make(chan struct{})
	runtime := deps.runtime(func(ctx context.Context, ready func()) error {
		ready()
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.runAttempt(ctx) }()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("session was not started")
	}
	if !runtime.Ready() {
		t.Fatal("runtime did not become ready")
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("runAttempt() error = %v, want context.Canceled", err)
	}
}

func TestClientRunAttemptFailureDoesNotSignalReady(t *testing.T) {
	runtime := (&clientTestDeps{}).runtime(func(context.Context, func()) error {
		return errors.New("connect failed")
	})

	err := runtime.runAttempt(context.Background())
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Fatalf("runAttempt() error = %v", err)
	}
	if runtime.Ready() {
		t.Fatal("runtime became ready after session creation failed")
	}
}

func TestClientRunReconnectsAfterSessionFailure(t *testing.T) {
	calls := 0
	runtime := (&clientTestDeps{}).runtime(func(context.Context, func()) error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	})

	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("session attempts = %d, want 2", calls)
	}
}

func TestClientRunStopsDuringReconnectDelay(t *testing.T) {
	runtime := (&clientTestDeps{}).runtime(func(context.Context, func()) error {
		return errors.New("transient")
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not stop after cancellation")
	}
}

func TestClientRunDisposesDevicesBeforeEachAttemptAndOnExit(t *testing.T) {
	deps := &clientTestDeps{disposeErr: errors.New("dispose error")}
	runtime := deps.runtime(func(context.Context, func()) error { return nil })

	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if deps.disposeCalls != 2 {
		t.Fatalf("DisposeDevices() calls = %d, want 2", deps.disposeCalls)
	}
}

func TestClientRunNormalizesCancellation(t *testing.T) {
	runtime := (&clientTestDeps{}).runtime(func(context.Context, func()) error {
		return context.Canceled
	})
	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want clean stop", err)
	}
}

func TestClientRunWithCanceledContext(t *testing.T) {
	runtime := (&clientTestDeps{}).runtime(func(context.Context, func()) error {
		t.Fatal("session must not start")
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
}
