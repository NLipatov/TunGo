package config

import (
	"path/filepath"
	"testing"
	"tungo/internal/platform"
)

func TestNewControls(t *testing.T) {
	controls := NewControls()
	if controls.Client == nil {
		t.Fatal("expected client control")
	}
	if got, want := controls.ServerSupported(), platform.ServerModeSupported(); got != want {
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
