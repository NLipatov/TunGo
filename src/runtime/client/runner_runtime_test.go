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

func TestRunSession_ClosesReadyAndRoutesTraffic(t *testing.T) {
	deps := &runtimeTestDeps{}
	routeStarted := make(chan struct{})
	r := NewRunner(deps, runtimeTestRouterFactory{
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
	readyCh := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- r.RunSession(ctx, SessionOptions{ReadyCh: readyCh})
	}()

	select {
	case <-readyCh:
	case <-time.After(time.Second):
		t.Fatal("ready channel was not closed")
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

func TestRunSession_CreateRouterErrorDoesNotCloseReady(t *testing.T) {
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
		create: func(context.Context, connection.Factory, tun.ClientManager, connection.ClientWorkerFactory) (routing.Router, connection.Transport, tun.Device, error) {
			return nil, nil, nil, errors.New("create failed")
		},
	})

	readyCh := make(chan struct{})
	err := r.RunSession(context.Background(), SessionOptions{ReadyCh: readyCh})
	if err == nil || !strings.Contains(err.Error(), "create failed") {
		t.Fatalf("expected create router error, got %v", err)
	}
	select {
	case <-readyCh:
		t.Fatal("ready channel must not close when router creation fails")
	default:
	}
}

func TestRun_CancelDuringReconnectDelay(t *testing.T) {
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
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
		t.Fatal("runner did not stop after context cancellation")
	}
}

func TestRun_ReconnectDelayAndRecovery(t *testing.T) {
	callCount := 0
	deps := &runtimeTestDeps{}
	r := NewRunner(deps, runtimeTestRouterFactory{
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
}

func TestRun_ContextAlreadyCanceled(t *testing.T) {
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

func TestRun_LogsDisposeErrorBranches(t *testing.T) {
	deps := &runtimeTestDeps{
		tun: runtimeTestTunManager{disposeErr: errors.New("dispose error")},
	}
	r := NewRunner(deps, runtimeTestRouterFactory{
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
