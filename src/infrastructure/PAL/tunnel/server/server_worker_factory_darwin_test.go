package server

import (
	"context"
	"io"
	"testing"

	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/settings"
)

type darwinNopReadWriteCloser struct{}

func (darwinNopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (darwinNopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (darwinNopReadWriteCloser) Close() error                { return nil }

func TestWorkerFactoryDarwin_NewAndAccessors(t *testing.T) {
	configuration := appConfiguration.ServerRuntimeConfiguration{}
	runtime, err := NewRuntime(configuration)
	if err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}

	f, err := NewWorkerFactory(runtime, configuration)
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
	configuration := appConfiguration.ServerRuntimeConfiguration{}
	runtime, err := NewRuntime(configuration)
	if err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}

	f, err := NewWorkerFactory(runtime, configuration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, workerErr := f.CreateWorker(context.Background(), darwinNopReadWriteCloser{}, settings.Settings{})
	if workerErr == nil {
		t.Fatal("expected error on unsupported platform")
	}
}
