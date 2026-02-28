package server

import (
	"context"
	"io"
	"testing"

	serverCfg "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type darwinDummyConfigManager struct{}

func (d *darwinDummyConfigManager) Configuration() (*serverCfg.Configuration, error) {
	return &serverCfg.Configuration{}, nil
}
func (d *darwinDummyConfigManager) AddAllowedPeer(_ serverCfg.AllowedPeer) error { return nil }
func (d *darwinDummyConfigManager) ListAllowedPeers() ([]serverCfg.AllowedPeer, error) {
	return nil, nil
}
func (d *darwinDummyConfigManager) SetAllowedPeerEnabled(_ int, _ bool) error { return nil }
func (d *darwinDummyConfigManager) RemoveAllowedPeer(_ int) error             { return nil }
func (d *darwinDummyConfigManager) IncrementClientCounter() error             { return nil }
func (d *darwinDummyConfigManager) InjectX25519Keys(_, _ []byte) error        { return nil }
func (d *darwinDummyConfigManager) EnsureIPv6Subnets() error                  { return nil }
func (d *darwinDummyConfigManager) InvalidateCache()                          {}

type darwinNopReadWriteCloser struct{}

func (darwinNopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (darwinNopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (darwinNopReadWriteCloser) Close() error                { return nil }

func TestWorkerFactoryDarwin_NewAndAccessors(t *testing.T) {
	mgr := &darwinDummyConfigManager{}
	runtime, err := NewRuntime(mgr)
	if err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}

	f, err := NewWorkerFactory(runtime, mgr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil factory")
	}
	if runtime.SessionRevoker() == nil {
		t.Fatal("expected non-nil session revoker")
	}
	if runtime.AllowedPeersUpdater() != nil {
		t.Fatal("expected nil allowed peers updater on darwin")
	}
}

func TestWorkerFactoryDarwin_CreateWorker_ReturnsError(t *testing.T) {
	mgr := &darwinDummyConfigManager{}
	runtime, err := NewRuntime(mgr)
	if err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}

	f, err := NewWorkerFactory(runtime, mgr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, workerErr := f.CreateWorker(context.Background(), darwinNopReadWriteCloser{}, settings.Settings{})
	if workerErr == nil {
		t.Fatal("expected error on unsupported platform")
	}
}
