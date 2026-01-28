package client_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	clientRunners "tungo/presentation/runners/client"
	"unsafe"
)

type dummyConnectionAdapter struct{}

func (d *dummyConnectionAdapter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (d *dummyConnectionAdapter) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (d *dummyConnectionAdapter) Close() error {
	return nil
}

type dummyTun struct{}

func (t *dummyTun) Write([]byte) (int, error) {
	return 0, nil
}

func (t *dummyTun) Read([]byte) (int, error) {
	return 0, nil
}

func (t *dummyTun) Close() error {
	return nil
}

// mockTunManager implements application.ClientManager.
type mockTunManager struct {
	disposeCount int
	disposeErr   error
}

func (d *mockTunManager) CreateDevice() (tun.Device, error) {
	return nil, nil
}

func (d *mockTunManager) DisposeDevices() error {
	d.disposeCount++
	return d.disposeErr
}

// mockConnectionFactory implements application.ConnectionFactory.
type mockConnectionFactory struct{}

func (d *mockConnectionFactory) EstablishConnection(_ context.Context,
) (connection.Transport, connection.Crypto, *rekey.StateMachine, error) {
	return nil, nil, nil, nil
}

// mockWorkerFactory implements application.ClientWorkerFactory.
type mockWorkerFactory struct{}

func (d *mockWorkerFactory) CreateWorker(
	_ context.Context, _ connection.Transport, _ io.ReadWriteCloser, _ connection.Crypto, _ *rekey.StateMachine,
) (routing.Worker, error) {
	return nil, nil
}

// mockRouter implements application.Router.
type mockRouter struct {
	routeCalled bool
	routeErr    error
}

func (d *mockRouter) RouteTraffic(ctx context.Context) error {
	d.routeCalled = true
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return d.routeErr
	}
}

// mockRouterFactory implements application.TrafficRouterFactory.
type mockRouterFactory struct {
	router routing.Router
	err    error
}

func (d *mockRouterFactory) CreateRouter(
	_ context.Context,
	_ connection.Factory,
	_ tun.ClientManager,
	_ connection.ClientWorkerFactory,
) (routing.Router, connection.Transport, tun.Device, error) {
	return d.router, &dummyConnectionAdapter{}, &dummyTun{}, d.err
}

// mockDeps implements presentation.ClientAppDependencies.
type mockDeps struct {
	conn   connection.Factory
	worker connection.ClientWorkerFactory
	tun    *mockTunManager
}

func (d *mockDeps) Initialize() error { return nil }
func (d *mockDeps) Configuration() client.Configuration {
	// Not used in ClientRunner.
	return client.Configuration{}
}
func (d *mockDeps) ConnectionFactory() connection.Factory         { return d.conn }
func (d *mockDeps) WorkerFactory() connection.ClientWorkerFactory { return d.worker }
func (d *mockDeps) TunManager() tun.ClientManager                 { return d.tun }

// setRouterBuilder sets the unexported routerBuilder field using unsafe.
func setRouterBuilder(runner *clientRunners.Runner, factory connection.TrafficRouterFactory) {
	v := reflect.ValueOf(runner).Elem().FieldByName("routerFactory")
	if !v.IsValid() {
		panic("routerFactory field not found")
	}
	ptrToField := unsafe.Pointer(v.UnsafeAddr())
	reflect.NewAt(v.Type(), ptrToField).Elem().Set(reflect.ValueOf(factory))
}

func TestClientRunner_Run_RouteTrafficCanceled(t *testing.T) {
	tunMgr := &mockTunManager{}
	deps := &mockDeps{
		conn:   &mockConnectionFactory{},
		worker: &mockWorkerFactory{},
		tun:    tunMgr,
	}
	router := &mockRouter{routeErr: context.Canceled}
	routerFactory := &mockRouterFactory{router: router}
	runner := clientRunners.NewRunner(deps, routerFactory)
	setRouterBuilder(runner, routerFactory)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runner.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()

	if tunMgr.disposeCount == 0 {
		t.Error("expected DisposeDevices to be called at least once")
	}
	if !router.routeCalled {
		t.Error("expected RouteTraffic to be called")
	}
}

func TestClientRunner_Run_CreateRouterError(t *testing.T) {
	tunMgr := &mockTunManager{}
	deps := &mockDeps{
		conn:   &mockConnectionFactory{},
		worker: &mockWorkerFactory{},
		tun:    tunMgr,
	}
	routerFactory := &mockRouterFactory{
		err: errors.New("create router error"),
	}
	runner := clientRunners.NewRunner(deps, routerFactory)
	setRouterBuilder(runner, routerFactory)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	runner.Run(ctx)

	if tunMgr.disposeCount == 0 {
		t.Error("expected DisposeDevices to be called even on router creation error")
	}
}
