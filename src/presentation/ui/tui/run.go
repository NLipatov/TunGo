package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	appConfiguration "tungo/application/configuration"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	appRuntime "tungo/runtime"
)

const runtimeLogCaptureCapacity = 1200

func (t *TUI) Run(ctx context.Context) error {
	if t.newRuntime == nil {
		return fmt.Errorf("runtime factory is nil")
	}

	bubbleTea.EnableGlobalRuntimeLogCapture(runtimeLogCaptureCapacity)
	defer bubbleTea.DisableGlobalRuntimeLogCapture()

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

	runtimeInstance, err := t.newRuntime(mode)
	if err != nil {
		return err
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	uiResultCh := make(chan RuntimeUIResult, 1)
	go func() {
		userQuit, err := t.runRuntimePhase(runtimeCtx, bubbleTea.RuntimeDashboardOptions{
			Mode:            mode,
			ServerSupported: t.sessionOptions.ServerSupported,
			LogFeed:         bubbleTea.GlobalRuntimeLogFeed(),
			WaitForReady:    runtimeInstance.WaitForReady,
			Protocol:        info.Protocol,
			Endpoints:       info.Endpoints,
		})
		cancel()
		uiResultCh <- RuntimeUIResult{UserQuit: userQuit, Err: err}
	}()

	workerErr := runtimeInstance.Run(runtimeCtx)
	cancel()
	uiResult := <-uiResultCh
	return resolveRuntimeEnd(
		uiResult,
		workerErr,
		func(err error) bool { return errors.Is(err, ErrUserExit) },
		func(err error) { slog.Error("runtime UI error", "err", err) },
	)
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
