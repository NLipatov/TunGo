package tui

import (
	"context"
	"errors"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type bubbleTeaRuntimeBackend struct{}

var (
	bubbleRuntimeEnableLogs   = bubbleTea.EnableGlobalRuntimeLogCapture
	bubbleRuntimeDisableLogs  = bubbleTea.DisableGlobalRuntimeLogCapture
	bubbleRuntimeRunDashboard = bubbleTea.RunRuntimeDashboard
	bubbleRuntimeLogFeed      = bubbleTea.GlobalRuntimeLogFeed
)

func newBubbleTeaRuntimeBackend() runtimeBackend {
	return bubbleTeaRuntimeBackend{}
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

	// Route to unified session when active (eliminates terminal flash).
	if activeUnifiedSession != nil {
		activeUnifiedSession.ActivateRuntime(ctx, options)
		reconfigure, err := activeUnifiedSession.WaitForRuntimeExit()
		if err != nil {
			if errors.Is(err, bubbleTea.ErrUnifiedSessionQuit) || errors.Is(err, bubbleTea.ErrUnifiedSessionClosed) {
				activeUnifiedSession.Close()
				activeUnifiedSession = nil
				return false, ErrUserExit
			}
			if errors.Is(err, bubbleTea.ErrUnifiedSessionRuntimeDisconnected) {
				// Runtime context cancelled (e.g. network error).
				// Session stays alive for the next ActivateRuntime call.
				return false, nil
			}
			activeUnifiedSession.Close()
			activeUnifiedSession = nil
			return false, err
		}
		return reconfigure, nil
	}

	// Fallback: standalone runtime dashboard (non-unified mode).
	reconfigureRequested, err := bubbleRuntimeRunDashboard(ctx, options)
	if err != nil {
		if errors.Is(err, bubbleTea.ErrRuntimeDashboardExitRequested) {
			return false, ErrUserExit
		}
		return false, err
	}
	return reconfigureRequested, nil
}
