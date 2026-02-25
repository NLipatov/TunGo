package server

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"tungo/application/network/routing"
	"tungo/domain/app"
	serverConfig "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
	runnerCommon "tungo/presentation/runners/common"
	"tungo/presentation/ui/tui"
)

func newTestServerRunner(deps AppDependencies, wf RunnerMockWorkerFactory, rf RunnerMockRouterFactory, dashboard RuntimeDashboardFunc) *Runner {
	r := NewRunner(app.TUI, deps, wf, rf)
	r.runRuntimeDashboard = dashboard
	return r
}

func blockingServerRouter() RunnerMockRouter {
	return RunnerMockRouter{
		route: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
}

func TestRun_Interactive_ReconfigureReturnsBackToModeSelection(t *testing.T) {
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
			return blockingServerRouter()
		},
	}

	r := newTestServerRunner(deps, wf, rf, func(context.Context, tui.RuntimeMode) (bool, error) {
		return true, nil
	})
	err := r.Run(context.Background())
	if !errors.Is(err, runnerCommon.ErrReconfigureRequested) {
		t.Fatalf("expected back-to-mode-selection from reconfigure request, got %v", err)
	}
}

func TestRun_Interactive_UIErrorWrappedWhenWorkersCanceled(t *testing.T) {
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
			return blockingServerRouter()
		},
	}

	r := newTestServerRunner(deps, wf, rf, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, errors.New("ui failed")
	})
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "runtime UI failed: ui failed") {
		t.Fatalf("expected wrapped runtime ui error, got %v", err)
	}
}

func TestRun_Interactive_UserExitErrorReturnsCanceled(t *testing.T) {
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
			return blockingServerRouter()
		},
	}

	r := newTestServerRunner(deps, wf, rf, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, tui.ErrUserExit
	})
	err := r.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from user exit, got %v", err)
	}
}

func TestRun_Interactive_WorkerErrorWinsOverUIError(t *testing.T) {
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

	r := newTestServerRunner(deps, wf, rf, func(ctx context.Context, _ tui.RuntimeMode) (bool, error) {
		<-ctx.Done()
		return false, errors.New("ui failed")
	})
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker failed") {
		t.Fatalf("expected worker error to win, got %v", err)
	}
}

func TestRun_Interactive_UIErrorReturnsWorkerErrWhenWorkerNotCanceled(t *testing.T) {
	uiStarted := make(chan struct{})
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
	r := newTestServerRunner(deps, wf, rf, func(context.Context, tui.RuntimeMode) (bool, error) {
		close(uiStarted)
		return false, errors.New("ui failed")
	})
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker explicit") {
		t.Fatalf("expected explicit worker error, got %v", err)
	}
}

func TestRun_Interactive_UserQuitReturnsWorkerErrWhenWorkerNotCanceled(t *testing.T) {
	uiStarted := make(chan struct{})
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
	r := newTestServerRunner(deps, wf, rf, func(context.Context, tui.RuntimeMode) (bool, error) {
		close(uiStarted)
		return true, nil
	})
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker explicit") {
		t.Fatalf("expected explicit worker error, got %v", err)
	}
}

func TestRun_Interactive_UICompletesWithoutQuitReturnsWorkerChannel(t *testing.T) {
	uiStarted := make(chan struct{})
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
	r := newTestServerRunner(deps, wf, rf, func(context.Context, tui.RuntimeMode) (bool, error) {
		close(uiStarted)
		return false, nil
	})
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "worker after ui") {
		t.Fatalf("expected worker channel result, got %v", err)
	}
}
