package tui

import (
	"context"
	"errors"
	"testing"
	bubbleTea "tungo/presentation/configuring/tui/components/implementations/bubble_tea"
)

type runtimeUITestFeed struct{}

func (runtimeUITestFeed) Tail(int) []string { return nil }
func (runtimeUITestFeed) TailInto([]string, int) int {
	return 0
}

func TestRuntimeUI_Wrappers(t *testing.T) {
	prevInteractive := bubbleIsInteractiveRuntime
	prevEnable := bubbleEnableLogCapture
	prevDisable := bubbleDisableLogCapture
	prevRun := bubbleRunRuntimeDashboard
	prevFeed := bubbleRuntimeLogFeed
	t.Cleanup(func() {
		bubbleIsInteractiveRuntime = prevInteractive
		bubbleEnableLogCapture = prevEnable
		bubbleDisableLogCapture = prevDisable
		bubbleRunRuntimeDashboard = prevRun
		bubbleRuntimeLogFeed = prevFeed
	})

	bubbleIsInteractiveRuntime = func() bool { return true }
	if !IsInteractiveRuntime() {
		t.Fatal("expected interactive runtime from wrapper")
	}

	var enabledCap int
	bubbleEnableLogCapture = func(capacity int) { enabledCap = capacity }
	EnableRuntimeLogCapture(42)
	if enabledCap != 42 {
		t.Fatalf("expected capture capacity 42, got %d", enabledCap)
	}

	disabled := false
	bubbleDisableLogCapture = func() { disabled = true }
	DisableRuntimeLogCapture()
	if !disabled {
		t.Fatal("expected disable wrapper to call implementation")
	}

	feed := runtimeUITestFeed{}
	bubbleRuntimeLogFeed = func() bubbleTea.RuntimeLogFeed { return feed }
	bubbleRunRuntimeDashboard = func(_ context.Context, options bubbleTea.RuntimeDashboardOptions) (bool, error) {
		if options.Mode != bubbleTea.RuntimeDashboardServer {
			t.Fatalf("expected server mode mapping, got %q", options.Mode)
		}
		if options.LogFeed != feed {
			t.Fatal("expected log feed to be forwarded")
		}
		return true, nil
	}
	quit, err := RunRuntimeDashboard(context.Background(), RuntimeModeServer)
	if err != nil || !quit {
		t.Fatalf("expected quit=true nil err, got quit=%v err=%v", quit, err)
	}

	bubbleRunRuntimeDashboard = func(_ context.Context, options bubbleTea.RuntimeDashboardOptions) (bool, error) {
		if options.Mode != bubbleTea.RuntimeDashboardClient {
			t.Fatalf("expected client mode mapping, got %q", options.Mode)
		}
		return false, errors.New("boom")
	}
	quit, err = RunRuntimeDashboard(context.Background(), RuntimeModeClient)
	if err == nil || quit {
		t.Fatalf("expected propagated error and quit=false, got quit=%v err=%v", quit, err)
	}
}
