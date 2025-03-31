package presentation_test

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"tungo/application"
	"tungo/presentation"
	"tungo/settings/client_configuration"
)

// mockTunManager implements application.TunManager.
type mockTunManager struct {
	disposeCount int
	disposeErr   error
}

func (d *mockTunManager) CreateTunDevice() (application.TunDevice, error) {
	return nil, nil
}

func (d *mockTunManager) DisposeTunDevices() error {
	d.disposeCount++
	return d.disposeErr
}

// mockConnectionFactory implements application.ConnectionFactory.
type mockConnectionFactory struct{}

func (d *mockConnectionFactory) EstablishConnection(_ context.Context,
) (net.Conn, application.CryptographyService, error) {
	return nil, nil, nil
}

// mockWorkerFactory implements application.TunWorkerFactory.
type mockWorkerFactory struct{}

func (d *mockWorkerFactory) CreateWorker(
	_ net.Conn, _ io.ReadWriteCloser, _ application.CryptographyService,
) (application.TunWorker, error) {
	return nil, nil
}

// mockRouter implements application.TrafficRouter.
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
	router application.TrafficRouter
	err    error
}

func (d *mockRouterFactory) CreateRouter(
	_ context.Context,
	_ application.ConnectionFactory,
	_ application.TunManager,
	_ application.TunWorkerFactory,
) (application.TrafficRouter, error) {
	return d.router, d.err
}

// mockDeps implements presentation.ClientAppDependencies.
type mockDeps struct {
	conn   application.ConnectionFactory
	worker application.TunWorkerFactory
	tun    *mockTunManager
}

func (d *mockDeps) Initialize() error { return nil }
func (d *mockDeps) Configuration() client_configuration.Configuration {
	// Not used in ClientRunner.
	return client_configuration.Configuration{}
}
func (d *mockDeps) ConnectionFactory() application.ConnectionFactory { return d.conn }
func (d *mockDeps) WorkerFactory() application.TunWorkerFactory      { return d.worker }
func (d *mockDeps) TunManager() application.TunManager               { return d.tun }

// setRouterBuilder sets the unexported routerBuilder field using unsafe.
func setRouterBuilder(runner *presentation.ClientRunner, factory application.TrafficRouterFactory) {
	v := reflect.ValueOf(runner).Elem().FieldByName("routerBuilder")
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
	runner := presentation.NewClientRunner(deps)
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
		t.Error("expected DisposeTunDevices to be called at least once")
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
	runner := presentation.NewClientRunner(deps)
	setRouterBuilder(runner, routerFactory)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	runner.Run(ctx)

	if tunMgr.disposeCount == 0 {
		t.Error("expected DisposeTunDevices to be called even on router creation error")
	}
}
