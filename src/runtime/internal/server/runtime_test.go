package server

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tungo/application/network/routing"
	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type runtimeResolver struct {
	path string
	err  error
}

func (r runtimeResolver) Resolve() (string, error) {
	return r.path, r.err
}

type runtimeConfigManager struct {
	cfg          *serverConf.Configuration
	cfgErr       error
	cfgErrOnCall int
	cfgCalls     int
	injectErr    error
}

func (m *runtimeConfigManager) Configuration() (*serverConf.Configuration, error) {
	m.cfgCalls++
	if m.cfgErrOnCall > 0 && m.cfgCalls == m.cfgErrOnCall {
		return nil, m.cfgErr
	}
	return m.cfg, nil
}
func (m *runtimeConfigManager) IncrementClientCounter() error { return nil }
func (m *runtimeConfigManager) InjectX25519Keys(_, _ []byte) error {
	return m.injectErr
}
func (m *runtimeConfigManager) AddAllowedPeer(serverConf.AllowedPeer) error {
	return nil
}
func (m *runtimeConfigManager) ListAllowedPeers() ([]serverConf.AllowedPeer, error) {
	return nil, nil
}
func (m *runtimeConfigManager) SetAllowedPeerEnabled(int, bool) error { return nil }
func (m *runtimeConfigManager) RemoveAllowedPeer(int) error           { return nil }
func (m *runtimeConfigManager) EnsureIPv6Subnets() error              { return nil }
func (m *runtimeConfigManager) InvalidateCache()                      {}

func TestSetupCrashLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")

	got := setupCrashLog(runtimeResolver{path: path})
	if got != path {
		t.Fatalf("setupCrashLog() = %q, want %q", got, path)
	}
}

func TestSetupCrashLog_ResolveError(t *testing.T) {
	got := setupCrashLog(runtimeResolver{err: errors.New("resolve failed")})
	if got != "" {
		t.Fatalf("expected empty crash log path on resolve error, got %q", got)
	}
}

func TestNewDefaultConfiguration(t *testing.T) {
	resolver, manager, err := NewDefaultConfiguration()
	if err != nil {
		t.Fatalf("NewDefaultConfiguration() error = %v", err)
	}
	if resolver == nil {
		t.Fatal("expected resolver")
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
}

func TestNewRuntime_PrepareKeysError(t *testing.T) {
	manager := &runtimeConfigManager{
		cfg:       &serverConf.Configuration{},
		injectErr: errors.New("inject failed"),
	}

	runtime, err := NewRuntime(
		runtimeResolver{path: filepath.Join(t.TempDir(), "server_configuration.json")},
		manager,
	)
	if runtime != nil {
		t.Fatalf("expected nil runtime, got %+v", runtime)
	}
	if err == nil || !strings.Contains(err.Error(), "key preparation failed") {
		t.Fatalf("expected key preparation error, got %v", err)
	}
}

func TestNewRuntime_ConfigurationError(t *testing.T) {
	manager := &runtimeConfigManager{
		cfg: &serverConf.Configuration{
			X25519PublicKey:  make([]byte, 32),
			X25519PrivateKey: make([]byte, 32),
		},
		cfgErr:       errors.New("read failed"),
		cfgErrOnCall: 2,
	}

	runtime, err := NewRuntime(
		runtimeResolver{path: filepath.Join(t.TempDir(), "server_configuration.json")},
		manager,
	)
	if runtime != nil {
		t.Fatalf("expected nil runtime, got %+v", runtime)
	}
	if err == nil || !strings.Contains(err.Error(), "failed to load server configuration") {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func TestNewRuntime_Success(t *testing.T) {
	manager := &runtimeConfigManager{
		cfg: &serverConf.Configuration{
			X25519PublicKey:  make([]byte, 32),
			X25519PrivateKey: make([]byte, 32),
		},
	}

	runtime, err := NewRuntime(
		runtimeResolver{path: filepath.Join(t.TempDir(), "server_configuration.json")},
		manager,
	)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime == nil {
		t.Fatal("expected runtime")
	}
}

func TestRuntimeRunDelegatesToRunner(t *testing.T) {
	want := errors.New("prepare failed")
	manager := &runtimeConfigManager{cfg: &serverConf.Configuration{}}
	runtime := &Runtime{
		runner: NewRunner(
			&RunnerMockDeps{
				key: &RunnerMockKeyManager{err: want},
				tun: &RunnerMockTunManager{},
				cfg: serverConf.Configuration{},
			},
			RunnerMockWorkerFactory{},
			RunnerMockRouterFactory{},
		),
		configWatcher: serverConf.NewConfigWatcher(
			manager,
			nil,
			nil,
			"",
			time.Hour,
			nil,
		),
	}

	err := runtime.Run(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("expected runner error, got %v", err)
	}
	if manager.cfgCalls == 0 {
		t.Fatal("expected Run to start and join the config watcher")
	}
}

func TestRuntimeRun_NormalizesCancellation(t *testing.T) {
	deps := &RunnerMockDeps{
		key: &RunnerMockKeyManager{},
		tun: &RunnerMockTunManager{},
		cfg: serverConf.Configuration{
			EnableTCP:   true,
			TCPSettings: settings.Settings{Protocol: settings.TCP},
		},
	}
	runtimeInstance := &Runtime{
		runner: NewRunner(
			deps,
			RunnerMockWorkerFactory{
				create: func(context.Context, io.ReadWriteCloser, settings.Settings) (routing.Worker, error) {
					return RunnerMockWorker{}, nil
				},
			},
			RunnerMockRouterFactory{
				make: func(routing.Worker) routing.Router {
					return RunnerMockRouter{
						route: func(context.Context) error { return context.Canceled },
					}
				},
			},
		),
	}

	if err := runtimeInstance.Run(context.Background()); err != nil {
		t.Fatalf("expected clean cancellation, got %v", err)
	}
}

func TestRuntimeRun_SuppressesLateFailureAfterContextCancellation(t *testing.T) {
	runtimeInstance := &Runtime{
		runner: NewRunner(
			&RunnerMockDeps{
				key: &RunnerMockKeyManager{err: errors.New("late failure")},
				tun: &RunnerMockTunManager{},
				cfg: serverConf.Configuration{},
			},
			RunnerMockWorkerFactory{},
			RunnerMockRouterFactory{},
		),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runtimeInstance.Run(ctx); err != nil {
		t.Fatalf("expected canceled context to win, got %v", err)
	}
}

func TestPrepareKeys_ExistingKeys(t *testing.T) {
	manager := &runtimeConfigManager{
		cfg: &serverConf.Configuration{
			X25519PublicKey:  make([]byte, 32),
			X25519PrivateKey: make([]byte, 32),
		},
	}

	if err := prepareKeys(manager); err != nil {
		t.Fatalf("prepareKeys() error = %v", err)
	}
}

func TestPrepareKeys_InjectError(t *testing.T) {
	manager := &runtimeConfigManager{
		cfg:       &serverConf.Configuration{},
		injectErr: errors.New("inject failed"),
	}

	err := prepareKeys(manager)
	if err == nil || !strings.Contains(err.Error(), "could not prepare keys") {
		t.Fatalf("expected wrapped key preparation error, got %v", err)
	}
}
