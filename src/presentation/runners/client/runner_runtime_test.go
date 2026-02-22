package client

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"strings"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	runnerCommon "tungo/presentation/runners/common"
	"tungo/presentation/ui/tui"
)

type runtimeTestTransport struct {
	closed int
}

func (t *runtimeTestTransport) Write(p []byte) (int, error) { return len(p), nil }
func (t *runtimeTestTransport) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (t *runtimeTestTransport) Close() error {
	t.closed++
	return nil
}

type runtimeTestTun struct {
	closed int
}

func (d *runtimeTestTun) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (d *runtimeTestTun) Write(p []byte) (int, error) { return len(p), nil }
func (d *runtimeTestTun) Close() error {
	d.closed++
	return nil
}

type runtimeTestTunManager struct {
	disposeCalls int
	disposeErr   error
}

func (m *runtimeTestTunManager) CreateDevice() (tun.Device, error) { return &runtimeTestTun{}, nil }
func (m *runtimeTestTunManager) DisposeDevices() error {
	m.disposeCalls++
	return m.disposeErr
}
func (m *runtimeTestTunManager) SetRouteEndpoint(netip.AddrPort) {}

type runtimeTestDeps struct {
	tun runtimeTestTunManager
}

func (d *runtimeTestDeps) Initialize() error                     { return nil }
func (d *runtimeTestDeps) Configuration() client.Configuration   { return client.Configuration{} }
func (d *runtimeTestDeps) ConnectionFactory() connection.Factory { return runtimeTestConnFactory{} }
func (d *runtimeTestDeps) WorkerFactory() connection.ClientWorkerFactory {
	return runtimeTestWorkerFactory{}
}
func (d *runtimeTestDeps) TunManager() tun.ClientManager { return &d.tun }

type runtimeTestConnFactory struct{}
type runtimeTestWorkerFactory struct{}

func (runtimeTestConnFactory) EstablishConnection(context.Context) (connection.Transport, connection.Crypto, *rekey.StateMachine, error) {
	return nil, nil, nil, nil
}
func (runtimeTestWorkerFactory) CreateWorker(context.Context, connection.Transport, io.ReadWriteCloser, connection.Crypto, *rekey.StateMachine) (routing.Worker, error) {
	return nil, nil
}

type runtimeTestRouter struct {
	route func(context.Context) error
}

func (r runtimeTestRouter) RouteTraffic(ctx context.Context) error {
	return r.route(ctx)
}

type runtimeTestRouterFactory struct {
	create func(ctx context.Context, connectionFactory connection.Factory, tunFactory tun.ClientManager, workerFactory connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error)
}

func (f runtimeTestRouterFactory) CreateRouter(
	ctx context.Context,
	connectionFactory connection.Factory,
	tunFactory tun.ClientManager,
	workerFactory connection.ClientWorkerFactory,
) (routing.Router, connection.Transport, tun.Device, error) {
	return f.create(ctx, connectionFactory, tunFactory, workerFactory)
}

func withClientRuntimeHooks(
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

func TestRunSession_Interactive_ReconfigureReturnsBackToModeSelection(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return true, nil
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(_ context.Context, _ connection.Factory, _ tun.ClientManager, _ connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			transport := &runtimeTestTransport{}
			device := &runtimeTestTun{}
			router := runtimeTestRouter{
				route: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}
			return router, transport, device, nil
		},
	})

	err := r.runSession(context.Background())
	if !errors.Is(err, runnerCommon.ErrReconfigureRequested) {
		t.Fatalf("expected back-to-mode-selection on reconfigure request, got %v", err)
	}
}

func TestRunSession_Interactive_UIErrorWrappedWhenRouteCanceled(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, errors.New("ui failed")
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(ctx context.Context, _ connection.Factory, _ tun.ClientManager, _ connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.runSession(context.Background())
	if err == nil || !strings.Contains(err.Error(), "runtime UI failed: ui failed") {
		t.Fatalf("expected wrapped ui error, got %v", err)
	}
}

func TestRunSession_Interactive_UserExitErrorCancelsSession(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, tui.ErrUserExit
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(_ context.Context, _ connection.Factory, _ tun.ClientManager, _ connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.runSession(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled on user exit, got %v", err)
	}
}

func TestRunSession_Interactive_RouteErrorWins(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, errors.New("ui failed")
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error {
					return errors.New("route failed")
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.runSession(context.Background())
	if err == nil || !strings.Contains(err.Error(), "route failed") {
		t.Fatalf("expected route error, got %v", err)
	}
}

func TestRunSession_NonInteractive_UsesRouterDirectly(t *testing.T) {
	withClientRuntimeHooks(t, false, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, nil
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error {
					return nil
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	if err := r.runSession(context.Background()); err != nil {
		t.Fatalf("expected nil route error, got %v", err)
	}
}

func TestRun_CancelDuringReconnectDelay(t *testing.T) {
	withClientRuntimeHooks(t, false, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, nil
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error { return errors.New("boom") },
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop after context cancellation")
	}
}

func TestRunSession_RouteErrorBranch_WaitsUIAndReturnsRouteErr(t *testing.T) {
	routeStarted := make(chan struct{})
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		<-routeStarted
		return false, errors.New("ui branch error")
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error {
					close(routeStarted)
					return errors.New("route early")
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.runSession(context.Background())
	if err == nil || !strings.Contains(err.Error(), "route early") {
		t.Fatalf("expected early route error, got %v", err)
	}
}

func TestRunSession_UserQuitReturnsRouteErrWhenNotCanceled(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return true, nil
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error {
					return errors.New("route explicit error")
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.runSession(context.Background())
	if err == nil || !strings.Contains(err.Error(), "route explicit error") {
		t.Fatalf("expected route error returned from user quit branch, got %v", err)
	}
}

func TestRunSession_UICompletesWithoutQuit_ReturnsRouteChannelResult(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, nil
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error {
					return errors.New("route after ui")
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.runSession(context.Background())
	if err == nil || !strings.Contains(err.Error(), "route after ui") {
		t.Fatalf("expected route channel result after UI completion, got %v", err)
	}
}

func TestRunSession_Interactive_UserExitError_RouteRealError_ReturnsRouteError(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, tui.ErrUserExit
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error {
					return errors.New("route real error")
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.runSession(context.Background())
	if err == nil || !strings.Contains(err.Error(), "route real error") {
		t.Fatalf("expected route real error, got %v", err)
	}
}

func TestRun_ReconnectDelayAndReconfigure(t *testing.T) {
	callCount := 0
	withClientRuntimeHooks(t, false, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, nil
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			callCount++
			router := runtimeTestRouter{
				route: func(context.Context) error {
					if callCount <= 1 {
						return errors.New("transient")
					}
					return nil
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil after recovery, got %v", err)
	}
	if callCount < 2 {
		t.Fatalf("expected at least 2 session attempts, got %d", callCount)
	}
}

func TestRun_ReconfigureRequestedPropagates(t *testing.T) {
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		return true, nil // reconfigure
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.Run(context.Background())
	if !errors.Is(err, runnerCommon.ErrReconfigureRequested) {
		t.Fatalf("expected ErrReconfigureRequested from Run, got %v", err)
	}
}

func TestRun_ContextAlreadyCanceled(t *testing.T) {
	withClientRuntimeHooks(t, false, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, nil
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return runtimeTestRouter{route: func(context.Context) error { return nil }}, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRunSession_Interactive_UIGenericError_RouteRealError_ReturnsRouteError(t *testing.T) {
	uiStarted := make(chan struct{})
	routeBlock := make(chan struct{})
	withClientRuntimeHooks(t, true, func(context.Context, tui.RuntimeMode) (bool, error) {
		close(uiStarted)
		return false, errors.New("ui generic error")
	})
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error {
					<-uiStarted
					<-routeBlock
					return errors.New("route real error")
				},
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- r.runSession(context.Background())
	}()
	// Allow route to finish after UI has returned.
	time.Sleep(50 * time.Millisecond)
	close(routeBlock)

	err := <-done
	if err == nil || !strings.Contains(err.Error(), "route real error") {
		t.Fatalf("expected route real error, got %v", err)
	}
}

func TestRun_LogsDisposeErrorBranches(t *testing.T) {
	withClientRuntimeHooks(t, false, func(context.Context, tui.RuntimeMode) (bool, error) {
		return false, nil
	})
	deps := &runtimeTestDeps{
		tun: runtimeTestTunManager{disposeErr: errors.New("dispose error")},
	}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			router := runtimeTestRouter{
				route: func(context.Context) error { return nil },
			}
			return router, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})
	r.Run(context.Background())
	if deps.tun.disposeCalls == 0 {
		t.Fatal("expected dispose to be called despite errors")
	}
}
