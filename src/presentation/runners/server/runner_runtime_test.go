package server

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
	"tungo/application/network/routing"
	serverConfig "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
	"tungo/presentation/configuring/tui"
)

func withServerRuntimeHooks(
	t *testing.T,
	interactive bool,
	runDashboard func(context.Context, tui.RuntimeMode) (bool, error),
) {
	t.Helper()
	prevInteractive := isInteractiveRuntime
	prevRunDashboard := runRuntimeDashboard
	isInteractiveRuntime = func() bool { return interactive }
	runRuntimeDashboard = runDashboard
	t.Cleanup(func() {
		isInteractiveRuntime = prevInteractive
		runRuntimeDashboard = prevRunDashboard
	})
}

func TestRun_Interactive_UserQuitReturnsCanceled(t *testing.T) {
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
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from user quit, got %v", err)
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

func TestRun_Interactive_WorkerErrorWinsOverUIError(t *testing.T) {
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		time.Sleep(15 * time.Millisecond)
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
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
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
					time.Sleep(15 * time.Millisecond)
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
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
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
					time.Sleep(15 * time.Millisecond)
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
	withServerRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
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
					time.Sleep(15 * time.Millisecond)
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
