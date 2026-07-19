package runtime

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
	"tungo/infrastructure/cryptography/chacha20/rekey"
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

func (d *runtimeTestDeps) newRuntime(routerFactory connection.TrafficRouterFactory) *clientRuntime {
	return &clientRuntime{
		connectionFactory: runtimeTestConnFactory{},
		workerFactory:     runtimeTestWorkerFactory{},
		tunManager:        &d.tun,
		routerFactory:     routerFactory,
		ready:             newReadySignal(),
	}
}

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

func TestRunAttempt_SignalsReadyAndRoutesTraffic(t *testing.T) {
	deps := &runtimeTestDeps{}
	routeStarted := make(chan struct{})
	r := deps.newRuntime(runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return runtimeTestRouter{
				route: func(ctx context.Context) error {
					close(routeStarted)
					<-ctx.Done()
					return ctx.Err()
				},
			}, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- r.runAttempt(ctx)
	}()

	readyCtx, cancelReady := context.WithTimeout(context.Background(), time.Second)
	defer cancelReady()
	if err := r.WaitForReady(readyCtx); err != nil {
		t.Fatalf("expected runtime to become ready, got %v", err)
	}
	select {
	case <-routeStarted:
	case <-time.After(time.Second):
		t.Fatal("route traffic was not started")
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRunAttempt_CreateRouterErrorDoesNotSignalReady(t *testing.T) {
	deps := &runtimeTestDeps{}
	r := deps.newRuntime(runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return nil, nil, nil, errors.New("create failed")
		},
	})

	err := r.runAttempt(context.Background())
	if err == nil || !strings.Contains(err.Error(), "create failed") {
		t.Fatalf("expected create router error, got %v", err)
	}
	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.WaitForReady(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected readiness wait to remain blocked, got %v", err)
	}
}

func TestRun_CancelDuringReconnectDelay(t *testing.T) {
	deps := &runtimeTestDeps{}
	r := deps.newRuntime(runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return runtimeTestRouter{
				route: func(context.Context) error { return errors.New("boom") },
			}, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = r.Run(ctx)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not stop after context cancellation")
	}
}

func TestRun_ReconnectDelayAndRecovery(t *testing.T) {
	callCount := 0
	deps := &runtimeTestDeps{}
	r := deps.newRuntime(runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			callCount++
			return runtimeTestRouter{
				route: func(context.Context) error {
					if callCount <= 1 {
						return errors.New("transient")
					}
					return nil
				},
			}, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil after recovery, got %v", err)
	}
	if callCount < 2 {
		t.Fatalf("expected at least 2 session attempts, got %d", callCount)
	}
	if err := r.WaitForReady(context.Background()); err != nil {
		t.Fatalf("expected runtime to report readiness, got %v", err)
	}
}

func TestRun_ContextAlreadyCanceled(t *testing.T) {
	deps := &runtimeTestDeps{}
	r := deps.newRuntime(runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return runtimeTestRouter{route: func(context.Context) error { return nil }}, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRun_LogsDisposeErrorBranches(t *testing.T) {
	deps := &runtimeTestDeps{
		tun: runtimeTestTunManager{disposeErr: errors.New("dispose error")},
	}
	r := deps.newRuntime(runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return runtimeTestRouter{
				route: func(context.Context) error { return nil },
			}, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})
	_ = r.Run(context.Background())
	if deps.tun.disposeCalls == 0 {
		t.Fatal("expected dispose to be called despite errors")
	}
}

func TestRuntimeRun_NormalizesCancellation(t *testing.T) {
	deps := &runtimeTestDeps{}
	runtimeInstance := deps.newRuntime(runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return runtimeTestRouter{
				route: func(context.Context) error { return context.Canceled },
			}, &runtimeTestTransport{}, &runtimeTestTun{}, nil
		},
	})

	if err := runtimeInstance.Run(context.Background()); err != nil {
		t.Fatalf("expected clean cancellation, got %v", err)
	}
}
