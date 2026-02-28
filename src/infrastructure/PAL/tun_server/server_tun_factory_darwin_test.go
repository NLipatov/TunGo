package tun_server

import (
	"testing"

	"tungo/infrastructure/settings"
)

func TestServerTunFactoryDarwin_New(t *testing.T) {
	f := NewServerTunFactory()
	if f == nil {
		t.Fatal("expected non-nil tun factory")
	}
}

func TestServerTunFactoryDarwin_CreateDevice_ReturnsError(t *testing.T) {
	f := ServerTunFactory{}
	_, err := f.CreateDevice(settings.Settings{})
	if err == nil {
		t.Fatal("expected error on unsupported platform")
	}
}

func TestServerTunFactoryDarwin_DisposeDevices_NoError(t *testing.T) {
	f := ServerTunFactory{}
	if err := f.DisposeDevices(settings.Settings{}); err != nil {
		t.Fatalf("expected nil error from DisposeDevices stub, got %v", err)
	}
}

