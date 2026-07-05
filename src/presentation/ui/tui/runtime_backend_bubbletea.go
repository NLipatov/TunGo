package tui

import (
	"context"
	"errors"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	"tungo/runtime"
)

type bubbleTeaRuntimeBackend struct {
	sh *sessionHolder
}

var (
	bubbleRuntimeEnableLogs   = bubbleTea.EnableGlobalRuntimeLogCapture
	bubbleRuntimeDisableLogs  = bubbleTea.DisableGlobalRuntimeLogCapture
	bubbleRuntimeRunDashboard = bubbleTea.RunRuntimeDashboard
	bubbleRuntimeLogFeed      = bubbleTea.GlobalRuntimeLogFeed
)

func newBubbleTeaRuntimeBackend() runtimeBackend {
	return &bubbleTeaRuntimeBackend{}
}

func (b *bubbleTeaRuntimeBackend) setSessionHolder(sh *sessionHolder) {
	b.sh = sh
}

func (b *bubbleTeaRuntimeBackend) enableRuntimeLogCapture(capacity int) {
	bubbleRuntimeEnableLogs(capacity)
}

func (b *bubbleTeaRuntimeBackend) disableRuntimeLogCapture() {
	bubbleRuntimeDisableLogs()
}

func (b *bubbleTeaRuntimeBackend) runRuntimeDashboard(ctx context.Context, mode runtime.Mode, options RuntimeUIOptions) (bool, error) {
	dashboardOptions := bubbleTea.RuntimeDashboardOptions{
		Mode:      mode,
		LogFeed:   bubbleRuntimeLogFeed(),
		ReadyCh:   options.ReadyCh,
		Protocol:  options.Protocol,
		Endpoints: options.Endpoints,
	}

	// Route to unified session when active (eliminates terminal flash).
	if b.sh != nil && b.sh.handle != nil {
		b.sh.handle.ActivateRuntime(ctx, dashboardOptions)
		reconfigure, err := b.sh.handle.WaitForRuntimeExit()
		if err != nil {
			if errors.Is(err, bubbleTea.ErrUnifiedSessionQuit) || errors.Is(err, bubbleTea.ErrUnifiedSessionClosed) {
				b.sh.handle.Close()
				b.sh.handle = nil
				return false, ErrUserExit
			}
			if errors.Is(err, bubbleTea.ErrUnifiedSessionRuntimeDisconnected) {
				// Runtime context cancelled (e.g. network error).
				// Session stays alive for the next ActivateRuntime call.
				return false, nil
			}
			b.sh.handle.Close()
			b.sh.handle = nil
			return false, err
		}
		return reconfigure, nil
	}

	// Fallback: standalone runtime dashboard (non-unified mode).
	reconfigureRequested, err := bubbleRuntimeRunDashboard(ctx, dashboardOptions)
	if err != nil {
		if errors.Is(err, bubbleTea.ErrRuntimeDashboardExitRequested) {
			return false, ErrUserExit
		}
		return false, err
	}
	return reconfigureRequested, nil
}
