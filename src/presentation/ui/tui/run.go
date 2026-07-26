package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	appConfiguration "tungo/application/configuration"
	appRuntime "tungo/application/runtime"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

func (t *TUI) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		runtimeMode, err := t.configure(ctx)
		if err != nil {
			if errors.Is(err, ErrUserExit) || ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, ErrSessionClosed) {
				return fmt.Errorf("ui session ended during shutdown: %w", err)
			}
			return fmt.Errorf("configuration error: %w", err)
		}

		err = t.runRuntime(ctx, runtimeMode)
		if errors.Is(err, errReconfigureRequested) {
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (t *TUI) runRuntime(ctx context.Context, mode appRuntime.Mode) error {
	info, err := t.runtimeInfo(mode)
	if err != nil {
		return fmt.Errorf("runtime info error: %w", err)
	}
	runtimeInstance, err := appRuntime.New(mode)
	if err != nil {
		return err
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	uiErrCh := make(chan error, 1)
	go func() {
		reconfigure, err := t.runRuntimePhase(runtimeCtx, bubbleTea.RuntimeDashboardOptions{
			Mode:      mode,
			Ready:     runtimeInstance.Ready,
			Protocol:  info.Protocol,
			Endpoints: info.Endpoints,
		})
		if err == nil && reconfigure {
			err = errReconfigureRequested
		}
		cancel()
		uiErrCh <- err
	}()

	workerErr := runtimeInstance.Run(runtimeCtx)
	cancel()
	uiErr := <-uiErrCh
	if uiErr != nil &&
		!errors.Is(uiErr, ErrUserExit) &&
		!errors.Is(uiErr, errReconfigureRequested) {
		slog.Error("runtime UI error", "err", uiErr)
	}
	if workerErr != nil {
		return workerErr
	}
	if uiErr == nil || errors.Is(uiErr, ErrUserExit) {
		return nil
	}
	if errors.Is(uiErr, errReconfigureRequested) {
		return errReconfigureRequested
	}
	return fmt.Errorf("runtime UI failed: %w", uiErr)
}

func (t *TUI) runtimeInfo(mode appRuntime.Mode) (appConfiguration.RuntimeInfo, error) {
	switch mode {
	case appRuntime.ModeClient:
		if t.sessionOptions.ClientConfigurationControl == nil {
			return appConfiguration.RuntimeInfo{}, fmt.Errorf("client configuration control is nil")
		}
		return t.sessionOptions.ClientConfigurationControl.RuntimeInfo()
	case appRuntime.ModeServer:
		if t.sessionOptions.ServerConfigurationControl == nil {
			return appConfiguration.RuntimeInfo{}, fmt.Errorf("server configuration control is nil")
		}
		return t.sessionOptions.ServerConfigurationControl.RuntimeInfo()
	default:
		return appConfiguration.RuntimeInfo{}, fmt.Errorf("invalid runtime mode: %v", mode)
	}
}

func (t *TUI) runRuntimePhase(
	ctx context.Context,
	options bubbleTea.RuntimeDashboardOptions,
) (bool, error) {
	if t.session == nil {
		return false, fmt.Errorf("runtime dashboard requires active tui session")
	}

	t.session.ActivateRuntime(ctx, options)
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
