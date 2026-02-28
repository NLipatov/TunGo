package server

import (
	"testing"

	"tungo/infrastructure/settings"
)

func TestTunFactoryDarwin_New(t *testing.T) {
	f := NewTunFactory()
	if f == nil {
		t.Fatal("expected non-nil tun factory")
	}
}

func TestTunFactoryDarwin_CreateDevice_ReturnsError(t *testing.T) {
	f := TunFactory{}
	_, err := f.CreateDevice(settings.Settings{})
	if err == nil {
		t.Fatal("expected error on unsupported platform")
	}
}

func TestTunFactoryDarwin_DisposeDevices_NoError(t *testing.T) {
	f := TunFactory{}
	if err := f.DisposeDevices(settings.Settings{}); err != nil {
		t.Fatalf("expected nil error from DisposeDevices stub, got %v", err)
	}
}

