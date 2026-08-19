package systemd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAvailable(t *testing.T) {
	t.Run("runtime directory missing", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if available(filepath.Join(t.TempDir(), "missing")) {
			t.Fatal("available() = true")
		}
	})

	t.Run("systemctl missing", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if available(t.TempDir()) {
			t.Fatal("available() = true")
		}
	})

	t.Run("available", func(t *testing.T) {
		binDir := t.TempDir()
		name := "systemctl"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", binDir)
		if !available(t.TempDir()) {
			t.Fatal("available() = false")
		}
	})
}
