package client

import (
	"path/filepath"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	t.Run("ProgramData", func(t *testing.T) {
		t.Setenv("ProgramData", `D:\Data`)
		expected := filepath.Join(`D:\Data`, "TunGo", "client_configuration.json")
		if actual := defaultPath(); actual != expected {
			t.Fatalf("defaultPath() = %q, want %q", actual, expected)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		t.Setenv("ProgramData", "")
		expected := filepath.Join(`C:\ProgramData`, "TunGo", "client_configuration.json")
		if actual := defaultPath(); actual != expected {
			t.Fatalf("defaultPath() = %q, want %q", actual, expected)
		}
	})
}
