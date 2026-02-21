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

	backend := bubbleTeaRuntimeBackend{}

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
