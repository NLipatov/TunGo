package configuration

import (
	"path/filepath"
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

func TestDefaultStorageDirectory(t *testing.T) {
	directory, err := DefaultStorageDirectory()
	if err != nil {
		t.Fatalf("DefaultStorageDirectory() error = %v", err)
	}
	if got, want := directory, filepath.Join(string(filepath.Separator), "etc", "tungo"); got != want {
		t.Fatalf("DefaultStorageDirectory() = %q, want %q", got, want)
	}
}
