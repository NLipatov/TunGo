package server

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	serverConf "tungo/infrastructure/PAL/configuration/server"
)

type runtimeResolver struct {
	path string
	err  error
}

func (r runtimeResolver) Resolve() (string, error) {
	return r.path, r.err
}

type runtimeConfigManager struct {
	cfg       *serverConf.Configuration
	injectErr error
}

func (m *runtimeConfigManager) Configuration() (*serverConf.Configuration, error) {
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

func TestRuntimeStop(t *testing.T) {
	stopped := false
	runtime := &Runtime{stopConfigWatcher: func() { stopped = true }}

	runtime.Stop()
	if !stopped {
		t.Fatal("expected Stop to cancel config watcher")
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
