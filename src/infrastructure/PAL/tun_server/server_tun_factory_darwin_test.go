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

func TestServerTunFactoryDarwin_CreateDevice_Panics(t *testing.T) {
	f := ServerTunFactory{}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_, _ = f.CreateDevice(settings.Settings{})
}

func TestServerTunFactoryDarwin_DisposeDevices_Panics(t *testing.T) {
	f := ServerTunFactory{}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = f.DisposeDevices(settings.Settings{})
}
