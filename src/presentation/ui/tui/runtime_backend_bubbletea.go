package tui

import (
	"context"
	"errors"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
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

// injectSessionHolder shares the Configurator's session holder with the
// active runtime backend. Both sides read/write the same holder.
func injectSessionHolder(sh *sessionHolder) {
	if bt, ok := activeRuntimeBackend.(*bubbleTeaRuntimeBackend); ok {
		bt.sh = sh
	}
}

func (b *bubbleTeaRuntimeBackend) enableRuntimeLogCapture(capacity int) {
	bubbleRuntimeEnableLogs(capacity)
}

func (b *bubbleTeaRuntimeBackend) disableRuntimeLogCapture() {
	bubbleRuntimeDisableLogs()
}

func (b *bubbleTeaRuntimeBackend) runRuntimeDashboard(ctx context.Context, mode RuntimeMode) (bool, error) {
	options := bubbleTea.RuntimeDashboardOptions{
		Mode:    bubbleTea.RuntimeDashboardClient,
		LogFeed: bubbleRuntimeLogFeed(),
	}
	if mode == RuntimeModeServer {
		options.Mode = bubbleTea.RuntimeDashboardServer
	}

	// Route to unified session when active (eliminates terminal flash).
	if b.sh != nil && b.sh.handle != nil {
		b.sh.handle.ActivateRuntime(ctx, options)
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
	reconfigureRequested, err := bubbleRuntimeRunDashboard(ctx, options)
	if err != nil {
		if errors.Is(err, bubbleTea.ErrRuntimeDashboardExitRequested) {
			return false, ErrUserExit
		}
		return false, err
	}
	return reconfigureRequested, nil
}
