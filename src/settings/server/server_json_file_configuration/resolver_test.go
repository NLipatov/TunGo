package server_json_file_configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSuccess(t *testing.T) {
	resolver := newResolver()
	actual, err := resolver.resolve()
	if err != nil {
		t.Fatalf("resolve() returned error: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() returned error: %v", err)
	}

	expected := filepath.Join(filepath.Dir(cwd), "src", "settings", "server", "conf.json")
	if actual != expected {
		t.Errorf("expected path %q, actual %q", expected, actual)
	}
}

// Test simulating error in os.Getwd() by changing to a directory that has been removed.
func TestResolveWorkingDirectoryError(t *testing.T) {
	// Create a temporary directory and change into it.
	tmpDir, err := os.MkdirTemp("", "testwd")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(origWD)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	// Remove the current directory to simulate os.Getwd() error.
	if err := os.Remove(tmpDir); err != nil {
		t.Fatalf("failed to remove temp dir: %v", err)
	}

	resolver := newResolver()
	_, err = resolver.resolve()
	if err == nil {
		t.Error("expected error from resolve() due to removed working directory, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to resolve configuration path") {
		t.Errorf("expected error message to contain 'failed to resolve configuration path', got %v", err)
	}
}
