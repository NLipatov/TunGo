package server

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"tungo/application/network/routing"
	serverConfig "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
	runnerCommon "tungo/presentation/runners/common"
	"tungo/presentation/ui/tui"
)

func withServerRuntimeHooks(
	t *testing.T,
	interactive bool,
	runDashboard func(context.Context, tui.RuntimeMode) (bool, error),
) {
	t.Helper()
	prevTUIMode := isTUIMode
	prevRunDashboard := runRuntimeDashboard
	isTUIMode = func() bool { return interactive }
	runRuntimeDashboard = runDashboard
	t.Cleanup(func() {
		isTUIMode = prevTUIMode
		runRuntimeDashboard = prevRunDashboard
	})
}

func TestRun_Interactive_ReconfigureReturnsBackToModeSelection(t *testing.T) {
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return true, nil
	})
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfig.Configuration{
			EnableUDP:   true,
			UDPSettings: settings.Settings{Protocol: settings.UDP},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{
				route: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}
		},
	}

	r := NewRunner(deps, wf, rf)
	err := r.Run(context.Background())
	if !errors.Is(err, runnerCommon.ErrReconfigureRequested) {
		t.Fatalf("expected back-to-mode-selection from reconfigure request, got %v", err)
	}
}

func TestRun_Interactive_UIErrorWrappedWhenWorkersCanceled(t *testing.T) {
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, errors.New("ui failed")
	})
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfig.Configuration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{
				route: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}
		},
	}

	r := NewRunner(deps, wf, rf)
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "runtime UI failed: ui failed") {
		t.Fatalf("expected wrapped runtime ui error, got %v", err)
	}
}

func TestRun_Interactive_UserExitErrorReturnsCanceled(t *testing.T) {
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, tui.ErrUserExit
	})
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfig.Configuration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{
				route: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}
		},
	}

	r := NewRunner(deps, wf, rf)
	err := r.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from user exit, got %v", err)
	}
}

func TestRun_Interactive_WorkerErrorWinsOverUIError(t *testing.T) {
	withServerRuntimeHooks(t, true, func(ctx context.Context, _ tui.RuntimeMode) (bool, error) {
		<-ctx.Done()
		return false, errors.New("ui failed")
	})
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfig.Configuration{
			EnableWS:   true,
			WSSettings: settings.Settings{Protocol: settings.WS},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{
				route: func(context.Context) error {
					return errors.New("worker failed fast")
				},
			}
		},
	}

	r := NewRunner(deps, wf, rf)
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker failed") {
		t.Fatalf("expected worker error to win, got %v", err)
	}
}

func TestRun_Interactive_UIErrorReturnsWorkerErrWhenWorkerNotCanceled(t *testing.T) {
	uiStarted := make(chan struct{})
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		close(uiStarted)
		return false, errors.New("ui failed")
	})
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfig.Configuration{
			EnableWS:   true,
			WSSettings: settings.Settings{Protocol: settings.WS},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{
				route: func(context.Context) error {
					<-uiStarted
					return errors.New("worker explicit")
				},
			}
		},
	}
	r := NewRunner(deps, wf, rf)
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker explicit") {
		t.Fatalf("expected explicit worker error, got %v", err)
	}
}

func TestRun_Interactive_UserQuitReturnsWorkerErrWhenWorkerNotCanceled(t *testing.T) {
	uiStarted := make(chan struct{})
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		close(uiStarted)
		return true, nil
	})
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfig.Configuration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{
				route: func(context.Context) error {
					<-uiStarted
					return errors.New("worker explicit")
				},
			}
		},
	}
	r := NewRunner(deps, wf, rf)
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker explicit") {
		t.Fatalf("expected explicit worker error, got %v", err)
	}
}

func TestRun_Interactive_UICompletesWithoutQuitReturnsWorkerChannel(t *testing.T) {
	uiStarted := make(chan struct{})
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		close(uiStarted)
		return false, nil
	})
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfig.Configuration{
			EnableUDP:   true,
			UDPSettings: settings.Settings{Protocol: settings.UDP},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{
				route: func(context.Context) error {
					<-uiStarted
					return errors.New("worker after ui")
				},
			}
		},
	}
	r := NewRunner(deps, wf, rf)
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker after ui") {
		t.Fatalf("expected worker channel result, got %v", err)
	}
}
