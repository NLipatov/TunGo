package tui

import (
	"context"
	"errors"
	"testing"
)

type runtimeUITestBackend struct {
	enabledCap int
	disabled   bool
	run        func(ctx context.Context, mode RuntimeMode) (bool, error)
}

func (m *runtimeUITestBackend) enableRuntimeLogCapture(capacity int) { m.enabledCap = capacity }

func (m *runtimeUITestBackend) disableRuntimeLogCapture() { m.disabled = true }

func (m *runtimeUITestBackend) runRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error) {
	if m.run == nil {
		return false, nil
	}
	return m.run(ctx, mode)
}

func TestRuntimeUI_Wrappers(t *testing.T) {
	prevBackend := activeRuntimeBackend
	t.Cleanup(func() {
		activeRuntimeBackend = prevBackend
	})

	mock := &runtimeUITestBackend{}
	activeRuntimeBackend = mock

	EnableRuntimeLogCapture(42)
	if mock.enabledCap != 42 {
		t.Fatalf("expected capture capacity 42, got %d", mock.enabledCap)
	}

	DisableRuntimeLogCapture()
	if !mock.disabled {
		t.Fatal("expected disable wrapper to call implementation")
	}

	mock.run = func(_ context.Context, mode RuntimeMode) (bool, error) {
		if mode != RuntimeModeServer {
			t.Fatalf("expected server mode mapping, got %q", mode)
		}
		return true, nil
	}
	quit, err := RunRuntimeDashboard(context.Background(), RuntimeModeServer)
	if err != nil || !quit {
		t.Fatalf("expected quit=true nil err, got quit=%v err=%v", quit, err)
	}

	mock.run = func(_ context.Context, mode RuntimeMode) (bool, error) {
		if mode != RuntimeModeClient {
			t.Fatalf("expected client mode mapping, got %q", mode)
		}
		return false, errors.New("boom")
	}
	quit, err = RunRuntimeDashboard(context.Background(), RuntimeModeClient)
	if err == nil || quit {
		t.Fatalf("expected propagated error and quit=false, got quit=%v err=%v", quit, err)
	}

	mock.run = func(_ context.Context, mode RuntimeMode) (bool, error) {
		if mode != RuntimeModeClient {
			t.Fatalf("expected client mode mapping, got %q", mode)
		}
		return false, ErrUserExit
	}
	quit, err = RunRuntimeDashboard(context.Background(), RuntimeModeClient)
	if !errors.Is(err, ErrUserExit) || quit {
		t.Fatalf("expected ErrUserExit and quit=false, got quit=%v err=%v", quit, err)
	}
}
