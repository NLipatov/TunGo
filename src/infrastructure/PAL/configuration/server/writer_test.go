package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "conf.json")
	w := newDefaultWriter(tmpFile)
	data := map[string]string{"key": "value"}

	if err := w.Write(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected, _ := json.MarshalIndent(data, "", "\t")
	if string(content) != string(expected) {
		t.Errorf("expected %s, got %s", expected, content)
	}
}

func TestJSONMarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "conf.json")
	w := newDefaultWriter(tmpFile)
	// Channels cannot be JSON-marshaled.
	ch := make(chan int)
	if err := w.Write(ch); err == nil {
		t.Error("expected error during JSON marshaling, got nil")
	}
}

func TestFileCreateError(t *testing.T) {
	// Passing an invalid path (contains a null byte) should trigger a file creation error.
	invalidPath := string([]byte{0})
	w := newDefaultWriter(invalidPath)
	if err := w.Write(map[string]string{"key": "value"}); err == nil {
		t.Error("expected file creation error, got nil")
	}
}

func TestFileWriteError(t *testing.T) {
	// Use /dev/full (on Unix) to simulate a write error.
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full not available, skipping test")
	}
	w := newDefaultWriter("/dev/full")
	if err := w.Write(map[string]string{"key": "value"}); err == nil {
		t.Error("expected file write error, got nil")
	}
}

func TestPathResolverSuccess(t *testing.T) {
	// NewServerResolver returns a fixed absolute path.
	resolver := NewServerResolver()
	resolved, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json")
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}

func TestMkdirAllError(t *testing.T) {
	// Create a temporary directory.
	tmpDir := t.TempDir()
	// Create a file that will be used in place of a directory.
	fakeDir := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(fakeDir, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	// Use a path inside fakeDir so that MkdirAll fails because fakeDir is not a directory.
	filePath := filepath.Join(fakeDir, "conf.json")
	w := newDefaultWriter(filePath)
	err := w.Write(map[string]string{"key": "value"})
	if err == nil {
		t.Fatal("expected error from MkdirAll, got nil")
	}

	// not a directory err is expected, because we used a fakeDir
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected error to mention 'not a directory', got %v", err)
	}
}
