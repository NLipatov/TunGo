package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"tungo/internal/client"
	"tungo/internal/mode"
	"tungo/internal/server"
	bubbleTea "tungo/internal/ui/tui/internal/bubble_tea"

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
	runtimeMode mode.Mode,
	logBuffer *bubbleTea.RuntimeLogBuffer,
) error {
	var running interface {
		Run(context.Context) error
		Ready() bool
	}
	var runtimeInfo func() bubbleTea.RuntimeInfo
	var err error
	switch runtimeMode {
	case mode.Client:
		runningClient, createErr := client.New(t.clientConfigurations)
		if createErr != nil {
			return createErr
		}
		running = runningClient
		runtimeInfo = func() bubbleTea.RuntimeInfo {
			configuration, err := t.clientConfigurations.Active()
			if err != nil {
				slog.Error("could not read active configuration", "err", err)
				return bubbleTea.RuntimeInfo{}
			}
			settings, err := configuration.ActiveSettings()
			if err != nil {
				slog.Error("could not read active settings", "err", err)
			}
			return clientRuntimeInfo(settings)
		}
	case mode.Server:
		if t.serverFile == nil {
			return fmt.Errorf("server configuration file is nil")
		}
		configuration, loadErr := t.serverFile.Load()
		if loadErr != nil {
			return fmt.Errorf("load server configuration: %w", loadErr)
		}
		info := serverRuntimeInfo(configuration)
		runtimeInfo = func() bubbleTea.RuntimeInfo { return info }
		running, err = server.New(t.serverFile)
	default:
		return fmt.Errorf("invalid runtime mode: %v", runtimeMode)
	}
	if err != nil {
		return err
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	uiErrCh := make(chan error, 1)
	go func() {
		reconfigure, err := t.runRuntimePhase(runtimeCtx, bubbleTea.RuntimeDashboardOptions{
			Mode:            runtimeMode,
			LogFeed:         logBuffer,
			ServerSupported: t.serverFile != nil,
			Ready:           running.Ready,
			RuntimeInfo:     runtimeInfo,
		})
		if err == nil && reconfigure {
			err = errReconfigureRequested
		}
		cancel()
		uiErrCh <- err
	}()

	workerErr := running.Run(runtimeCtx)
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
	if options.Mode == mode.Client {
		if err := t.preferences.DisableAutoConnect(); err != nil {
			return false, fmt.Errorf("persist AutoConnect=false: %w", err)
		}
	}
	return true, nil
}
