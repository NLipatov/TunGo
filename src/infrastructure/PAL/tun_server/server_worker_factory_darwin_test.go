package tun_server

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
func (d *darwinDummyConfigManager) IncrementClientCounter() error                { return nil }
func (d *darwinDummyConfigManager) InjectX25519Keys(_, _ []byte) error           { return nil }
func (d *darwinDummyConfigManager) InvalidateCache()                             {}

type darwinNopReadWriteCloser struct{}

func (darwinNopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (darwinNopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (darwinNopReadWriteCloser) Close() error                { return nil }

func TestServerWorkerFactoryDarwin_NewAndAccessors(t *testing.T) {
	f, err := NewServerWorkerFactory(&darwinDummyConfigManager{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil factory")
	}
	if f.SessionRevoker() == nil {
		t.Fatal("expected non-nil session revoker")
	}
	if f.AllowedPeersUpdater() != nil {
		t.Fatal("expected nil allowed peers updater on darwin")
	}
}

func TestServerWorkerFactoryDarwin_CreateWorker_Panics(t *testing.T) {
	f, err := NewServerWorkerFactory(&darwinDummyConfigManager{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()

	_, _ = f.CreateWorker(context.Background(), darwinNopReadWriteCloser{}, settings.Settings{})
}
