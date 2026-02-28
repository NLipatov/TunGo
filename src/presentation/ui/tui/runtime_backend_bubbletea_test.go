package tui

import (
	"context"
	"errors"
	"testing"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type runtimeBackendTestFeed struct{}

func (runtimeBackendTestFeed) Tail(int) []string { return nil }

func (runtimeBackendTestFeed) TailInto([]string, int) int { return 0 }

func TestBubbleTeaRuntimeBackend_MappingAndHooks(t *testing.T) {
	prevEnable := bubbleRuntimeEnableLogs
	prevDisable := bubbleRuntimeDisableLogs
	prevRun := bubbleRuntimeRunDashboard
	prevFeed := bubbleRuntimeLogFeed
	t.Cleanup(func() {
		bubbleRuntimeEnableLogs = prevEnable
		bubbleRuntimeDisableLogs = prevDisable
		bubbleRuntimeRunDashboard = prevRun
		bubbleRuntimeLogFeed = prevFeed
	})

	backend := &bubbleTeaRuntimeBackend{}

	capacity := 0
	bubbleRuntimeEnableLogs = func(v int) { capacity = v }
	backend.enableRuntimeLogCapture(64)
	if capacity != 64 {
		t.Fatalf("expected capture capacity 64, got %d", capacity)
	}

	disabled := false
	bubbleRuntimeDisableLogs = func() { disabled = true }
	backend.disableRuntimeLogCapture()
	if !disabled {
		t.Fatal("expected disable call")
	}

	feed := runtimeBackendTestFeed{}
	bubbleRuntimeLogFeed = func() bubbleTea.RuntimeLogFeed { return feed }
	bubbleRuntimeRunDashboard = func(_ context.Context, options bubbleTea.RuntimeDashboardOptions) (bool, error) {
		if options.Mode != bubbleTea.RuntimeDashboardServer {
			t.Fatalf("expected server mode mapping, got %q", options.Mode)
		}
		if options.LogFeed != feed {
			t.Fatal("expected runtime log feed to be forwarded")
		}
		return true, nil
	}
	reconfigure, err := backend.runRuntimeDashboard(context.Background(), RuntimeModeServer)
	if err != nil || !reconfigure {
		t.Fatalf("expected reconfigure=true nil err, got reconfigure=%v err=%v", reconfigure, err)
	}

	bubbleRuntimeRunDashboard = func(_ context.Context, options bubbleTea.RuntimeDashboardOptions) (bool, error) {
		if options.Mode != bubbleTea.RuntimeDashboardClient {
			t.Fatalf("expected client mode mapping, got %q", options.Mode)
		}
		return false, errors.New("boom")
	}
	reconfigure, err = backend.runRuntimeDashboard(context.Background(), RuntimeModeClient)
	if err == nil || reconfigure {
		t.Fatalf("expected propagated error and reconfigure=false, got reconfigure=%v err=%v", reconfigure, err)
	}

	bubbleRuntimeRunDashboard = func(_ context.Context, options bubbleTea.RuntimeDashboardOptions) (bool, error) {
		if options.Mode != bubbleTea.RuntimeDashboardClient {
			t.Fatalf("expected client mode mapping, got %q", options.Mode)
		}
		return false, bubbleTea.ErrRuntimeDashboardExitRequested
	}
	reconfigure, err = backend.runRuntimeDashboard(context.Background(), RuntimeModeClient)
	if !errors.Is(err, ErrUserExit) || reconfigure {
		t.Fatalf("expected ErrUserExit and reconfigure=false, got reconfigure=%v err=%v", reconfigure, err)
	}
}

func backendWithSession(session unifiedSessionHandle) *bubbleTeaRuntimeBackend {
	return &bubbleTeaRuntimeBackend{
		sh: &sessionHolder{handle: session},
	}
}

func TestRunRuntimeDashboard_UnifiedSession_HappyPath_Reconfigure(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeReconfigure: true}
	backend := backendWithSession(mock)

	reconfigure, err := backend.runRuntimeDashboard(context.Background(), RuntimeModeServer)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reconfigure {
		t.Fatal("expected reconfigure=true")
	}
	if !mock.activateCalled {
		t.Fatal("expected ActivateRuntime called")
	}
}

func TestRunRuntimeDashboard_UnifiedSession_Quit_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: bubbleTea.ErrUnifiedSessionQuit}
	backend := backendWithSession(mock)

	reconfigure, err := backend.runRuntimeDashboard(context.Background(), RuntimeModeClient)
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called")
	}
	if backend.sh.handle != nil {
		t.Fatal("expected session cleared")
	}
}

func TestRunRuntimeDashboard_UnifiedSession_Closed_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: bubbleTea.ErrUnifiedSessionClosed}
	backend := backendWithSession(mock)

	reconfigure, err := backend.runRuntimeDashboard(context.Background(), RuntimeModeClient)
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called")
	}
	if backend.sh.handle != nil {
		t.Fatal("expected session cleared")
	}
}

func TestRunRuntimeDashboard_UnifiedSession_Disconnected_KeepsSession(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: bubbleTea.ErrUnifiedSessionRuntimeDisconnected}
	backend := backendWithSession(mock)

	reconfigure, err := backend.runRuntimeDashboard(context.Background(), RuntimeModeServer)
	if err != nil {
		t.Fatalf("expected nil error for disconnect, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false for disconnect")
	}
	if mock.closeCalled {
		t.Fatal("expected Close NOT called on disconnect")
	}
	if backend.sh.handle == nil {
		t.Fatal("expected session preserved on disconnect")
	}
}

func TestRunRuntimeDashboard_UnifiedSession_GenericError_ClearsSession(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: errors.New("unexpected")}
	backend := backendWithSession(mock)

	reconfigure, err := backend.runRuntimeDashboard(context.Background(), RuntimeModeClient)
	if err == nil || err.Error() != "unexpected" {
		t.Fatalf("expected 'unexpected', got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called")
	}
	if backend.sh.handle != nil {
		t.Fatal("expected session cleared")
	}
}

func TestRunRuntimeDashboard_UnifiedSession_NoError_ReturnsReconfigure(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeReconfigure: false}
	backend := backendWithSession(mock)

	reconfigure, err := backend.runRuntimeDashboard(context.Background(), RuntimeModeServer)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
}

func TestInjectSessionHolder(t *testing.T) {
	prevBackend := activeRuntimeBackend
	t.Cleanup(func() { activeRuntimeBackend = prevBackend })

	backend := &bubbleTeaRuntimeBackend{}
	activeRuntimeBackend = backend

	sh := &sessionHolder{handle: &mockUnifiedSession{}}
	injectSessionHolder(sh)

	if backend.sh != sh {
		t.Fatal("expected session holder injected into backend")
	}
}
