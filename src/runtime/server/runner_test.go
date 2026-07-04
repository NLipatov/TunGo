package server

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
	"tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	"tungo/domain/app"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

// -------------------- shared test data --------------------

var errBoom = errors.New("boom")

// -------------------- AppDependencies test doubles --------------------

type RunnerMockKeyManager struct {
	err   error
	calls int32
}

func (m *RunnerMockKeyManager) PrepareKeys() error {
	atomic.AddInt32(&m.calls, 1)
	return m.err
}

// Simple io.ReadWriteCloser representing a TUN handle.
type RunnerMockTun struct {
	closed int32
}

func (t *RunnerMockTun) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (t *RunnerMockTun) Write(p []byte) (int, error) { return len(p), nil }
func (t *RunnerMockTun) Close() error {
	atomic.AddInt32(&t.closed, 1)
	return nil
}

// Implements application.ServerManager.
type RunnerMockTunManager struct {
	createErrByProto map[settings.Protocol]error
	disposeErr       error

	createCalls    int32
	disposeCalls   int32
	lastCreatedTun *RunnerMockTun
}

func (m *RunnerMockTunManager) CreateDevice(s settings.Settings) (tun.Device, error) {
	atomic.AddInt32(&m.createCalls, 1)
	if e := m.createErrByProto[s.Protocol]; e != nil {
		return nil, e
	}
	t := &RunnerMockTun{}
	m.lastCreatedTun = t
	return t, nil
}

func (m *RunnerMockTunManager) DisposeDevices(_ settings.Settings) error {
	atomic.AddInt32(&m.disposeCalls, 1)
	return m.disposeErr
}

// Matches server.AppDependencies structurally.
type RunnerMockDeps struct {
	key  *RunnerMockKeyManager
	tun  *RunnerMockTunManager
	cfg  server.Configuration
	cmgr any
}

func (d *RunnerMockDeps) KeyManager() server.KeyManager { return d.key }

// IMPORTANT: return the named interface expected by AppDependencies.
func (d *RunnerMockDeps) TunManager() tun.ServerManager { return d.tun }

// IMPORTANT: return by value per AppDependencies.
func (d *RunnerMockDeps) Configuration() server.Configuration { return d.cfg }

func (d *RunnerMockDeps) ConfigurationManager() server.ConfigurationManager {
	panic("not implemented")
}

// -------------------- application.* DI test doubles --------------------

// Worker per your interface.
type RunnerMockWorker struct {
	handleTunErr       error
	handleTransportErr error
}

func (w RunnerMockWorker) HandleTun() error       { return w.handleTunErr }
func (w RunnerMockWorker) HandleTransport() error { return w.handleTransportErr }

// ServerWorkerFactory mock.
type RunnerMockWorkerFactory struct {
	create func(ctx context.Context, tun io.ReadWriteCloser, s settings.Settings) (routing.Worker, error)
}

func (f RunnerMockWorkerFactory) CreateWorker(ctx context.Context, tun io.ReadWriteCloser, s settings.Settings) (routing.Worker, error) {
	return f.create(ctx, tun, s)
}

// Router mock.
type RunnerMockRouter struct {
	route func(context.Context) error
}

func (r RunnerMockRouter) RouteTraffic(ctx context.Context) error { return r.route(ctx) }

// ServerTrafficRouterFactory mock.
type RunnerMockRouterFactory struct {
	make func(routing.Worker) routing.Router
}

func (f RunnerMockRouterFactory) CreateRouter(w routing.Worker) routing.Router {
	return f.make(w)
}

// -------------------- tests --------------------

func TestRun_Happy_AllProtocols(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: server.Configuration{
			EnableTCP:   true,
			EnableUDP:   true,
			EnableWS:    true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
			UDPSettings: settings.Settings{Protocol: settings.UDP},
			WSSettings:  settings.Settings{Protocol: settings.WS},
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
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(1 * time.Millisecond):
						return nil
					}
				},
			}
		},
	}
	r := NewRunner(app.CLI, deps, wf, rf)

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got := atomic.LoadInt32(&deps.key.calls); got != 1 {
		t.Fatalf("PrepareKeys calls=%d want=1", got)
	}
	if got := atomic.LoadInt32(&deps.tun.createCalls); got != 3 {
		t.Fatalf("CreateDevice calls=%d want=3", got)
	}
	// cleanup runs pre+post over 3 settings => 6
	if got := atomic.LoadInt32(&deps.tun.disposeCalls); got != 6 {
		t.Fatalf("DisposeDevices calls=%d want=6", got)
	}
}

func TestRun_KeyManagerError(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{err: errBoom},
		tun: &RunnerMockTunManager{},
		cfg: server.Configuration{},
	}
	r := NewRunner(app.CLI, deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	if err := r.Run(context.Background()); err == nil || err.Error() != "failed to generate ed25519 keys: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_NoProtocolsEnabled(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: server.Configuration{}, // all disabled
	}
	r := NewRunner(app.CLI, deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	// Current implementation: no error when no protocols enabled.
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	// No CreateDevice calls because no protocols enabled
	if got := atomic.LoadInt32(&deps.tun.createCalls); got != 0 {
		t.Fatalf("CreateDevice calls=%d want=0", got)
	}
	// cleanup still runs pre + post over 3 settings => 6
	if got := atomic.LoadInt32(&deps.tun.disposeCalls); got != 6 {
		t.Fatalf("DisposeDevices calls=%d want=6", got)
	}
}

func TestRun_WorkerFlagsMatrix(t *testing.T) {
	cases := []struct {
		name         string
		tcp, udp, ws bool
		wantCreates  int32
	}{
		{"tcp", true, false, false, 1},
		{"udp", false, true, false, 1},
		{"ws", false, false, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := &RunnerMockDeps{
				key: &RunnerMockKeyManager{},
				tun: &RunnerMockTunManager{},
				cfg: server.Configuration{
					EnableTCP:   tc.tcp,
					EnableUDP:   tc.udp,
					EnableWS:    tc.ws,
					TCPSettings: settings.Settings{Protocol: settings.TCP},
					UDPSettings: settings.Settings{Protocol: settings.UDP},
					WSSettings:  settings.Settings{Protocol: settings.WS},
				},
			}
			r := NewRunner(app.CLI, deps,
				RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
					return RunnerMockWorker{}, nil
				}},
				RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
					return RunnerMockRouter{route: func(context.Context) error { return nil }}
				}},
			)
			if err := r.Run(context.Background()); err != nil {
				t.Fatalf("Run error: %v", err)
			}
			if got := atomic.LoadInt32(&deps.tun.createCalls); got != tc.wantCreates {
				t.Fatalf("CreateDevice calls=%d want=%d", got, tc.wantCreates)
			}
			if got := atomic.LoadInt32(&deps.tun.disposeCalls); got != 6 {
				t.Fatalf("DisposeDevices calls=%d want=6", got)
			}
		})
	}
}

func TestCleanup_ErrorAggregates(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{disposeErr: errBoom},
		cfg: server.Configuration{
			TCPSettings: settings.Settings{Protocol: settings.TCP},
			UDPSettings: settings.Settings{Protocol: settings.UDP},
			WSSettings:  settings.Settings{Protocol: settings.WS},
		},
	}
	r := NewRunner(app.CLI, deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	if err := r.cleanup(); err == nil {
		t.Fatalf("expected non-nil error from cleanup when DisposeDevices fails")
	}
}

func TestRoute_CreateTunError(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errBoom},
		},
		cfg: server.Configuration{},
	}
	r := NewRunner(app.CLI, deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	_, err := r.createRouter(context.Background(), settings.Settings{Protocol: settings.TCP})
	if err == nil || err.Error() != "error creating tun device: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoute_CreateWorkerError(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: server.Configuration{},
	}
	r := NewRunner(app.CLI, deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return nil, errBoom
		}},
		RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	_, err := r.createRouter(context.Background(), settings.Settings{Protocol: settings.WS})
	if err == nil || err.Error() != "error creating worker: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
	if deps.tun.lastCreatedTun == nil {
		t.Fatal("expected created tun")
	}
	if got := atomic.LoadInt32(&deps.tun.lastCreatedTun.closed); got != 1 {
		t.Fatalf("expected tun to be closed on worker creation error, got closed=%d", got)
	}
}

func TestRunWorkers_SingleRouteError(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: server.Configuration{
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
			return RunnerMockRouter{route: func(context.Context) error { return errBoom }}
		},
	}
	r := NewRunner(app.CLI, deps, wf, rf)
	err := r.runWorkers(context.Background())
	if err == nil {
		t.Fatalf("expected error from runWorkers, got nil")
	}
	// error message should indicate worker failure and contain underlying error
	if !contains(err.Error(), "worker failed") || !contains(err.Error(), "boom") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRunWorkers_AggregatesMultipleErrors(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: server.Configuration{
			EnableTCP:   true,
			EnableUDP:   true, // two workers fail
			TCPSettings: settings.Settings{Protocol: settings.TCP},
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
			return RunnerMockRouter{route: func(context.Context) error { return errBoom }}
		},
	}

	r := NewRunner(app.CLI, deps, wf, rf)
	err := r.runWorkers(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()
	if !(contains(msg, "tcp") || contains(msg, "udp") || contains(msg, "worker failed")) {
		t.Fatalf("unexpected aggregated error: %v", err)
	}
}

func TestRun_CleanupError_ContinuesRunning(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{disposeErr: errBoom},
		cfg: server.Configuration{}, // no protocols — runWorkers succeeds immediately
	}
	r := NewRunner(app.CLI, deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	// cleanup() errors are logged, Run() still succeeds.
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run should succeed despite cleanup error: %v", err)
	}
}

func TestRunWorkers_CreateRouterError(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{
			createErrByProto: map[settings.Protocol]error{settings.TCP: errBoom},
		},
		cfg: server.Configuration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	}
	r := NewRunner(app.CLI, deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(routing.Worker) routing.Router {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	err := r.runWorkers(context.Background())
	if err == nil || !contains(err.Error(), "could not create") {
		t.Fatalf("expected createRouter error, got: %v", err)
	}
}

// tiny substring helper
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
