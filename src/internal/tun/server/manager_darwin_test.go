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

func TestTunFactoryDarwin_Create_ReturnsError(t *testing.T) {
	f := Manager{}
	_, err := f.Create(settings.Settings{})
	if err == nil {
		t.Fatal("expected error on unsupported platform")
	}
}

func TestTunFactoryDarwin_Remove_NoError(t *testing.T) {
	f := Manager{}
	if err := f.Remove(settings.Settings{}); err != nil {
		t.Fatalf("expected nil error from Remove stub, got %v", err)
	}
}
