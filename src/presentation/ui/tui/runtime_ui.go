package tui

import (
	"context"
	"errors"
	"tungo/infrastructure/settings"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	"tungo/runtime"
)

type RuntimeUIOptions struct {
	ReadyCh   <-chan struct{}
	Endpoints []runtime.EndpointInfo
	Protocol  settings.Protocol
}

func (t *TUI) EnableRuntimeLogCapture(capacity int) {
	bubbleTea.EnableGlobalRuntimeLogCapture(capacity)
}

func (t *TUI) DisableRuntimeLogCapture() {
	bubbleTea.DisableGlobalRuntimeLogCapture()
}

func (t *TUI) RunRuntimeDashboard(ctx context.Context, mode runtime.Mode, options RuntimeUIOptions) (bool, error) {
	dashboardOptions := bubbleTea.RuntimeDashboardOptions{
		Mode:      mode,
		LogFeed:   bubbleTea.GlobalRuntimeLogFeed(),
		ReadyCh:   options.ReadyCh,
		Protocol:  options.Protocol,
		Endpoints: options.Endpoints,
	}

	if t.session != nil {
		t.session.ActivateRuntime(ctx, dashboardOptions)
		reconfigure, err := t.session.WaitForRuntimeExit()
		if err != nil {
			if errors.Is(err, bubbleTea.ErrUnifiedSessionQuit) || errors.Is(err, bubbleTea.ErrUnifiedSessionClosed) {
				t.closeSession()
				return false, ErrUserExit
			}
			if errors.Is(err, bubbleTea.ErrUnifiedSessionRuntimeDisconnected) {
				return false, nil
			}
			t.closeSession()
			return false, err
		}
		return reconfigure, nil
	}

	reconfigureRequested, err := bubbleTea.RunRuntimeDashboard(ctx, dashboardOptions)
	if err != nil {
		if errors.Is(err, bubbleTea.ErrRuntimeDashboardExitRequested) {
			return false, ErrUserExit
		}
		return false, err
	}
	return reconfigureRequested, nil
}
