package server

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
	"tungo/application/network/tun"

	"tungo/application"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
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

	createCalls  int32
	disposeCalls int32
}

func (m *RunnerMockTunManager) CreateDevice(s settings.Settings) (tun.Device, error) {
	atomic.AddInt32(&m.createCalls, 1)
	if e := m.createErrByProto[s.Protocol]; e != nil {
		return nil, e
	}
	return &RunnerMockTun{}, nil
}

func (m *RunnerMockTunManager) DisposeDevices(_ settings.Settings) error {
	atomic.AddInt32(&m.disposeCalls, 1)
	return m.disposeErr
}

// Matches server.AppDependencies structurally.
type RunnerMockDeps struct {
	key  *RunnerMockKeyManager
	tun  *RunnerMockTunManager
	cfg  serverConfiguration.Configuration
	cmgr any
}

func (d *RunnerMockDeps) KeyManager() serverConfiguration.KeyManager { return d.key }

// IMPORTANT: return the named interface expected by AppDependencies.
func (d *RunnerMockDeps) TunManager() tun.ServerManager { return d.tun }

// IMPORTANT: return by value per AppDependencies.
func (d *RunnerMockDeps) Configuration() serverConfiguration.Configuration { return d.cfg }

func (d *RunnerMockDeps) ConfigurationManager() serverConfiguration.ServerConfigurationManager {
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
	create func(ctx context.Context, tun io.ReadWriteCloser, s settings.Settings) (tun.Worker, error)
}

func (f RunnerMockWorkerFactory) CreateWorker(ctx context.Context, tun io.ReadWriteCloser, s settings.Settings) (tun.Worker, error) {
	return f.create(ctx, tun, s)
}

// TrafficRouter mock.
type RunnerMockRouter struct {
	route func(context.Context) error
}

func (r RunnerMockRouter) RouteTraffic(ctx context.Context) error { return r.route(ctx) }

// ServerTrafficRouterFactory mock.
type RunnerMockRouterFactory struct {
	make func(tun.Worker) application.TrafficRouter
}

func (f RunnerMockRouterFactory) CreateRouter(w tun.Worker) application.TrafficRouter {
	return f.make(w)
}

// -------------------- tests --------------------

func TestRun_Happy_AllProtocols(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfiguration.Configuration{
			EnableTCP:   true,
			EnableUDP:   true,
			EnableWS:    true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
			UDPSettings: settings.Settings{Protocol: settings.UDP},
			WSSettings:  settings.Settings{Protocol: settings.WS},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(tun.Worker) application.TrafficRouter {
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
	r := NewRunner(deps, wf, rf)

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
		cfg: serverConfiguration.Configuration{},
	}
	r := NewRunner(deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(tun.Worker) application.TrafficRouter {
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
		cfg: serverConfiguration.Configuration{}, // all disabled
	}
	r := NewRunner(deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(tun.Worker) application.TrafficRouter {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	err := r.Run(context.Background())
	if err == nil || err.Error() != "no protocol is enabled in server configuration" {
		t.Fatalf("unexpected error: %v", err)
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
				cfg: serverConfiguration.Configuration{
					EnableTCP:   tc.tcp,
					EnableUDP:   tc.udp,
					EnableWS:    tc.ws,
					TCPSettings: settings.Settings{Protocol: settings.TCP},
					UDPSettings: settings.Settings{Protocol: settings.UDP},
					WSSettings:  settings.Settings{Protocol: settings.WS},
				},
			}
			r := NewRunner(deps,
				RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
					return RunnerMockWorker{}, nil
				}},
				RunnerMockRouterFactory{make: func(tun.Worker) application.TrafficRouter {
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
		cfg: serverConfiguration.Configuration{
			TCPSettings: settings.Settings{Protocol: settings.TCP},
			UDPSettings: settings.Settings{Protocol: settings.UDP},
			WSSettings:  settings.Settings{Protocol: settings.WS},
		},
	}
	r := NewRunner(deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(tun.Worker) application.TrafficRouter {
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
		cfg: serverConfiguration.Configuration{},
	}
	r := NewRunner(deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(tun.Worker) application.TrafficRouter {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	err := r.route(context.Background(), settings.Settings{Protocol: settings.TCP})
	if err == nil || err.Error() != "error creating tun device: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoute_CreateWorkerError(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfiguration.Configuration{},
	}
	r := NewRunner(deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return nil, errBoom
		}},
		RunnerMockRouterFactory{make: func(tun.Worker) application.TrafficRouter {
			return RunnerMockRouter{route: func(context.Context) error { return nil }}
		}},
	)
	err := r.route(context.Background(), settings.Settings{Protocol: settings.WS})
	if err == nil || err.Error() != "error creating worker: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoute_RouteTrafficError(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfiguration.Configuration{},
	}
	r := NewRunner(deps,
		RunnerMockWorkerFactory{create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return RunnerMockWorker{}, nil
		}},
		RunnerMockRouterFactory{make: func(tun.Worker) application.TrafficRouter {
			return RunnerMockRouter{route: func(context.Context) error { return errBoom }}
		}},
	)
	err := r.route(context.Background(), settings.Settings{Protocol: settings.UDP})
	if err == nil || err.Error() != "error routing traffic: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWorkers_AggregatesMultipleErrors(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConfiguration.Configuration{
			EnableTCP:   true,
			EnableUDP:   true, // two workers fail
			TCPSettings: settings.Settings{Protocol: settings.TCP},
			UDPSettings: settings.Settings{Protocol: settings.UDP},
		},
	}
	wf := RunnerMockWorkerFactory{
		create: func(context.Context, io.ReadWriteCloser, settings.Settings) (tun.Worker, error) {
			return RunnerMockWorker{}, nil
		},
	}
	rf := RunnerMockRouterFactory{
		make: func(tun.Worker) application.TrafficRouter {
			return RunnerMockRouter{route: func(context.Context) error { return errBoom }}
		},
	}

	r := NewRunner(deps, wf, rf)
	err := r.runWorkers(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()
	if !(contains(msg, "tcp") || contains(msg, "udp") || contains(msg, "worker failed")) {
		t.Fatalf("unexpected aggregated error: %v", err)
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
