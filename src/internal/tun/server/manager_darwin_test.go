package server

import (
	"testing"

	"tungo/internal/config/settings"
)

func TestTunFactoryDarwin_New(t *testing.T) {
	f := New()
	if f == nil {
		t.Fatal("expected non-nil tun factory")
	}
}

func TestTunFactoryDarwin_Open_ReturnsError(t *testing.T) {
	f := Manager{}
	_, err := f.Open(settings.Settings{})
	if err == nil {
		t.Fatal("expected error on unsupported platform")
	}
}

func TestTunFactoryDarwin_Close_NoError(t *testing.T) {
	f := Manager{}
	if err := f.Close(settings.Settings{}); err != nil {
		t.Fatalf("expected nil error from Close stub, got %v", err)
	}
}
