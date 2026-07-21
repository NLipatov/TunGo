package configuration

import (
	"testing"
	"tungo/infrastructure/PAL/platform"
)

func TestNewDefaultControls(t *testing.T) {
	controls := NewDefaultControls()
	if controls.Client == nil {
		t.Fatal("expected client control")
	}
	if got, want := controls.ServerSupported(), platform.Capabilities().ServerModeSupported(); got != want {
		t.Fatalf("ServerSupported() = %v, want %v", got, want)
	}
}
