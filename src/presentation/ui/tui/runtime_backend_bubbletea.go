package tui

import (
	"context"
	"errors"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type bubbleTeaRuntimeBackend struct{}

var (
	bubbleRuntimeIsInteractive = bubbleTea.IsInteractiveTerminal
	bubbleRuntimeEnableLogs    = bubbleTea.EnableGlobalRuntimeLogCapture
	bubbleRuntimeDisableLogs   = bubbleTea.DisableGlobalRuntimeLogCapture
	bubbleRuntimeRunDashboard  = bubbleTea.RunRuntimeDashboard
	bubbleRuntimeLogFeed       = bubbleTea.GlobalRuntimeLogFeed
)

func newBubbleTeaRuntimeBackend() runtimeBackend {
	return bubbleTeaRuntimeBackend{}
}

func (bubbleTeaRuntimeBackend) isInteractiveTerminal() bool {
	return bubbleRuntimeIsInteractive()
}

func (bubbleTeaRuntimeBackend) enableRuntimeLogCapture(capacity int) {
	bubbleRuntimeEnableLogs(capacity)
}

func (bubbleTeaRuntimeBackend) disableRuntimeLogCapture() {
	bubbleRuntimeDisableLogs()
}

func (bubbleTeaRuntimeBackend) runRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error) {
	options := bubbleTea.RuntimeDashboardOptions{
		Mode:    bubbleTea.RuntimeDashboardClient,
		LogFeed: bubbleRuntimeLogFeed(),
	}
	if mode == RuntimeModeServer {
		options.Mode = bubbleTea.RuntimeDashboardServer
	}
	reconfigureRequested, err := bubbleRuntimeRunDashboard(ctx, options)
	if err != nil {
		if errors.Is(err, bubbleTea.ErrRuntimeDashboardExitRequested) {
			return false, ErrUserExit
		}
		return false, err
	}
	return reconfigureRequested, nil
}
