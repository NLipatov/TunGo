package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"tungo/application"
	appConfiguration "tungo/application/configuration"
	tunnelClient "tungo/infrastructure/tunnel/client"
	tunnelServer "tungo/infrastructure/tunnel/server"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"

	tea "charm.land/bubbletea/v2"
)

const runtimeLogCaptureCapacity = 1200

func (t *TUI) Run(ctx context.Context) error {
	logBuffer := bubbleTea.NewRuntimeLogBuffer(runtimeLogCaptureCapacity)
	restoreLogger := bubbleTea.RedirectStandardLoggerToBuffer(logBuffer)
	defer restoreLogger()

	for ctx.Err() == nil {
		runtimeMode, err := t.configure(ctx, logBuffer)
		if err != nil {
			if errors.Is(err, ErrUserExit) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("configuration error: %w", err)
		}

		err = t.runRuntime(ctx, runtimeMode, logBuffer)
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

func (t *TUI) runRuntime(
	ctx context.Context,
	mode application.Mode,
	logBuffer *bubbleTea.RuntimeLogBuffer,
) error {
	info, err := t.runtimeInfo(mode)
	if err != nil {
		return fmt.Errorf("runtime info error: %w", err)
	}
	var runtimeInstance interface {
		Run(context.Context) error
		Ready() bool
	}
	switch mode {
	case application.ModeClient:
		runtimeInstance, err = tunnelClient.New()
	case application.ModeServer:
		runtimeInstance, err = tunnelServer.New()
	default:
		return fmt.Errorf("invalid runtime mode: %v", mode)
	}
	if err != nil {
		return err
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	uiErrCh := make(chan error, 1)
	go func() {
		reconfigure, err := t.runRuntimePhase(runtimeCtx, bubbleTea.RuntimeDashboardOptions{
			Mode:            mode,
			LogFeed:         logBuffer,
			ServerSupported: t.configuratorOptions.ServerConfigurationControl != nil,
			Ready:           runtimeInstance.Ready,
			Protocol:        info.Protocol,
			Endpoints:       info.Endpoints,
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
	if uiErr != nil && !errors.Is(uiErr, errReconfigureRequested) {
		slog.Error("runtime UI error", "err", uiErr)
	}
	if workerErr != nil {
		logBuffer.WriteSeparator("disconnected")
		return workerErr
	}
	if uiErr == nil {
		return nil
	}
	if errors.Is(uiErr, errReconfigureRequested) {
		logBuffer.WriteSeparator("reconfigured")
		return errReconfigureRequested
	}
	return fmt.Errorf("runtime UI failed: %w", uiErr)
}

func (t *TUI) runtimeInfo(mode application.Mode) (appConfiguration.RuntimeInfo, error) {
	switch mode {
	case application.ModeClient:
		if t.configuratorOptions.ClientConfigurationControl == nil {
			return appConfiguration.RuntimeInfo{}, fmt.Errorf("client configuration control is nil")
		}
		return t.configuratorOptions.ClientConfigurationControl.RuntimeInfo()
	case application.ModeServer:
		if t.configuratorOptions.ServerConfigurationControl == nil {
			return appConfiguration.RuntimeInfo{}, fmt.Errorf("server configuration control is nil")
		}
		return t.configuratorOptions.ServerConfigurationControl.RuntimeInfo()
	default:
		return appConfiguration.RuntimeInfo{}, fmt.Errorf("invalid runtime mode: %v", mode)
	}
}

func (t *TUI) runRuntimePhase(
	ctx context.Context,
	options bubbleTea.RuntimeDashboardOptions,
) (bool, error) {
	model := bubbleTea.NewRuntimeDashboard(ctx, options, t.preferences)
	program := tea.NewProgram(model)
	stopQuit := context.AfterFunc(ctx, program.Quit)
	finalModel, err := program.Run()
	stopQuit()
	if err != nil {
		return false, err
	}
	dashboard, ok := finalModel.(bubbleTea.RuntimeDashboard)
	if !ok {
		return false, fmt.Errorf("unexpected runtime dashboard model: %T", finalModel)
	}
	if !dashboard.ReconfigureRequested() {
		return false, nil
	}
	if options.Mode == application.ModeClient {
		if err := t.preferences.DisableAutoConnect(); err != nil {
			return false, fmt.Errorf("persist AutoConnect=false: %w", err)
		}
	}
	return true, nil
}
