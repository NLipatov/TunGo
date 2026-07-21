package runtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"tungo/application/configuration"
	"tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

var errServerTest = errors.New("boom")

type serverTestTun struct {
	closed int32
}

func (t *serverTestTun) Read([]byte) (int, error)    { return 0, io.EOF }
func (t *serverTestTun) Write(p []byte) (int, error) { return len(p), nil }
func (t *serverTestTun) Close() error {
	atomic.AddInt32(&t.closed, 1)
	return nil
}

type serverTestTunManager struct {
	createErrByProto map[settings.Protocol]error
	disposeErr       error
	createCalls      int32
	disposeCalls     int32
	lastCreatedTun   *serverTestTun
}

func (m *serverTestTunManager) CreateDevice(s settings.Settings) (tun.Device, error) {
	atomic.AddInt32(&m.createCalls, 1)
	if err := m.createErrByProto[s.Protocol]; err != nil {
		return nil, err
	}
	device := &serverTestTun{}
	m.lastCreatedTun = device
	return device, nil
}

func (m *serverTestTunManager) DisposeDevices(settings.Settings) error {
	atomic.AddInt32(&m.disposeCalls, 1)
	return m.disposeErr
}

type serverTestWorker struct{}

func (serverTestWorker) HandleTun() error       { return nil }
func (serverTestWorker) HandleTransport() error { return nil }

type serverTestWorkerFactory struct {
	create func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error)
}

func (f serverTestWorkerFactory) CreateWorker(
	ctx context.Context,
	device io.ReadWriteCloser,
	settings settings.Settings,
) (routing.Worker, error) {
	if f.create != nil {
		return f.create(ctx, device, settings)
	}
	return serverTestWorker{}, nil
}

type serverTestRouter struct {
	route func(context.Context) error
}

func (r serverTestRouter) RouteTraffic(ctx context.Context) error {
	if r.route != nil {
		return r.route(ctx)
	}
	return nil
}

type serverTestRouterFactory struct {
	make func(routing.Worker) routing.Router
}

func (f serverTestRouterFactory) CreateRouter(worker routing.Worker) routing.Router {
	if f.make != nil {
		return f.make(worker)
	}
	return serverTestRouter{}
}

func newServerTestRuntime(
	manager *serverTestTunManager,
	config configuration.ServerRuntimeConfiguration,
) *serverRuntime {
	return &serverRuntime{
		config:        config,
		tunManager:    manager,
		workerFactory: serverTestWorkerFactory{},
		routerFactory: serverTestRouterFactory{},
		control:       &serverTestControl{},
	}
}

func TestServerRun_AllProtocols(t *testing.T) {
	manager := &serverTestTunManager{}
	runtime := newServerTestRuntime(manager, configuration.ServerRuntimeConfiguration{
		EnableTCP:   true,
		EnableUDP:   true,
		EnableWS:    true,
		TCPSettings: settings.Settings{Protocol: settings.TCP},
		UDPSettings: settings.Settings{Protocol: settings.UDP},
		WSSettings:  settings.Settings{Protocol: settings.WS},
	})

	if err := runtime.run(context.Background()); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !runtime.Ready() {
		t.Fatal("runtime did not become ready")
	}
	if got := atomic.LoadInt32(&manager.createCalls); got != 3 {
		t.Fatalf("CreateDevice() calls = %d, want 3", got)
	}
	if got := atomic.LoadInt32(&manager.disposeCalls); got != 6 {
		t.Fatalf("DisposeDevices() calls = %d, want 6", got)
	}
}

func TestServerRun_NoProtocolsEnabled(t *testing.T) {
	manager := &serverTestTunManager{}
	runtime := newServerTestRuntime(manager, configuration.ServerRuntimeConfiguration{})

	if err := runtime.run(context.Background()); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := atomic.LoadInt32(&manager.createCalls); got != 0 {
		t.Fatalf("CreateDevice() calls = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&manager.disposeCalls); got != 6 {
		t.Fatalf("DisposeDevices() calls = %d, want 6", got)
	}
}

func TestServerCleanup_ReturnsDisposeError(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{disposeErr: errServerTest},
		configuration.ServerRuntimeConfiguration{},
	)

	if err := runtime.cleanup(); !errors.Is(err, errServerTest) {
		t.Fatalf("cleanup() error = %v, want %v", err, errServerTest)
	}
}

func TestServerCreateRouter_TunError(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errServerTest},
		},
		configuration.ServerRuntimeConfiguration{},
	)

	_, err := runtime.createRouter(context.Background(), settings.Settings{Protocol: settings.TCP})
	if !errors.Is(err, errServerTest) || !strings.Contains(err.Error(), "creating tun device") {
		t.Fatalf("createRouter() error = %v", err)
	}
}

func TestServerCreateRouter_ClosesTunAfterWorkerError(t *testing.T) {
	manager := &serverTestTunManager{}
	runtime := newServerTestRuntime(manager, configuration.ServerRuntimeConfiguration{})
	runtime.workerFactory = serverTestWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return nil, errServerTest
		},
	}

	_, err := runtime.createRouter(context.Background(), settings.Settings{Protocol: settings.WS})
	if !errors.Is(err, errServerTest) {
		t.Fatalf("createRouter() error = %v, want %v", err, errServerTest)
	}
	if got := atomic.LoadInt32(&manager.lastCreatedTun.closed); got != 1 {
		t.Fatalf("tun Close() calls = %d, want 1", got)
	}
}

func TestServerRunWorkers_WrapsRouteError(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{},
		configuration.ServerRuntimeConfiguration{
			EnableUDP:   true,
			UDPSettings: settings.Settings{Protocol: settings.UDP},
		},
	)
	runtime.routerFactory = serverTestRouterFactory{
		make: func(routing.Worker) routing.Router {
			return serverTestRouter{route: func(context.Context) error { return errServerTest }}
		},
	}

	err := runtime.runWorkers(context.Background())
	if !errors.Is(err, errServerTest) || !strings.Contains(err.Error(), "worker failed") {
		t.Fatalf("runWorkers() error = %v", err)
	}
}

func TestServerRunWorkers_WrapsRouterCreationError(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errServerTest},
		},
		configuration.ServerRuntimeConfiguration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	)

	err := runtime.runWorkers(context.Background())
	if !errors.Is(err, errServerTest) || !strings.Contains(err.Error(), "could not create") {
		t.Fatalf("runWorkers() error = %v", err)
	}
}

func TestServerRun_IgnoresCleanupError(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{disposeErr: errServerTest},
		configuration.ServerRuntimeConfiguration{},
	)

	if err := runtime.run(context.Background()); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

type serverTestControl struct {
	watchStarted int32
}

func (*serverTestControl) ServerRuntimeConfiguration() (configuration.ServerRuntimeConfiguration, error) {
	return configuration.ServerRuntimeConfiguration{}, nil
}

func (c *serverTestControl) WatchServerRuntimeConfiguration(
	ctx context.Context,
	_ configuration.ServerSessionRevoker,
	_ configuration.ServerAllowedPeersUpdater,
) {
	atomic.AddInt32(&c.watchStarted, 1)
	<-ctx.Done()
}

func TestServerRun_StartsWatcher(t *testing.T) {
	control := &serverTestControl{}
	runtime := newServerTestRuntime(
		&serverTestTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errServerTest},
		},
		configuration.ServerRuntimeConfiguration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	)
	runtime.control = control

	err := runtime.Run(context.Background())
	if !errors.Is(err, errServerTest) {
		t.Fatalf("Run() error = %v, want %v", err, errServerTest)
	}
	if atomic.LoadInt32(&control.watchStarted) == 0 {
		t.Fatal("configuration watcher was not started")
	}
}

func TestServerRun_NormalizesCancellation(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{},
		configuration.ServerRuntimeConfiguration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	)
	runtime.workerFactory = serverTestWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return serverTestWorker{}, nil
		},
	}
	runtime.routerFactory = serverTestRouterFactory{
		make: func(routing.Worker) routing.Router {
			return serverTestRouter{route: func(context.Context) error { return context.Canceled }}
		},
	}

	if err := runtime.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want clean stop", err)
	}
}

func TestServerRun_CanceledContextWinsOverRuntimeError(t *testing.T) {
	runtime := newServerTestRuntime(
		&serverTestTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errServerTest},
		},
		configuration.ServerRuntimeConfiguration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runtime.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v, want clean stop", err)
	}
}
