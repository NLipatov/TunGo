package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetCrashOutput_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")

	SetCrashOutput(path)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected crash log to be created: %v", err)
	}
}

func TestSetCrashOutput_AppendsSeparatorWhenFileHasContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")
	if err := os.WriteFile(path, []byte("previous crash"), 0600); err != nil {
		t.Fatalf("write crash log fixture: %v", err)
	}

	SetCrashOutput(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read crash log: %v", err)
	}
	if !strings.Contains(string(data), "--- crash at ") {
		t.Fatalf("expected crash separator, got %q", string(data))
	}
}

func TestSetCrashOutput_OpenErrorIsIgnored(t *testing.T) {
	SetCrashOutput(filepath.Join(t.TempDir(), "missing", "crash.log"))
}
