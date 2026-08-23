package server

import (
	"testing"

	"tungo/internal/config/settings"
)

func TestTunFactoryDarwin_New(t *testing.T) {
	f := NewManager()
	if f == nil {
		t.Fatal("expected non-nil tun factory")
	}
}

func TestTunFactoryDarwin_OpenTunnel_ReturnsError(t *testing.T) {
	f := Manager{}
	_, err := f.OpenTunnel(settings.Settings{})
	if err == nil {
		t.Fatal("expected error on unsupported platform")
	}
}

func TestTunFactoryDarwin_CloseTunnel_NoError(t *testing.T) {
	f := Manager{}
	if err := f.CloseTunnel(settings.Settings{}); err != nil {
		t.Fatalf("expected nil error from CloseTunnel stub, got %v", err)
	}
}
