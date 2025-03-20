package server_json_file_configuration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type writerTestMockResolver struct {
	path string
	err  error
}

func (f writerTestMockResolver) resolve() (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.path, nil
}

func TestWriteSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "conf.json")
	w := newWriter(writerTestMockResolver{path: tmpFile})
	data := map[string]string{"key": "value"}
	if err := w.Write(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected, _ := json.MarshalIndent(data, "", "  ")
	if string(content) != string(expected) {
		t.Errorf("expected %s, got %s", expected, content)
	}
}

func TestJSONMarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "conf.json")
	w := newWriter(writerTestMockResolver{path: tmpFile})
	ch := make(chan int)
	if err := w.Write(ch); err == nil {
		t.Error("expected error during JSON marshaling, got nil")
	}
}

func TestResolverError(t *testing.T) {
	expectedErr := errors.New("resolver error")
	w := newWriter(writerTestMockResolver{err: expectedErr})
	if err := w.Write(map[string]string{"key": "value"}); err == nil {
		t.Error("expected resolver error, got nil")
	} else if err.Error() != expectedErr.Error() {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestFileCreateError(t *testing.T) {
	invalidPath := string([]byte{0})
	w := newWriter(writerTestMockResolver{path: invalidPath})
	if err := w.Write(map[string]string{"key": "value"}); err == nil {
		t.Error("expected file creation error, got nil")
	}
}

func TestFileWriteError(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full not available, skipping test")
	}
	w := newWriter(writerTestMockResolver{path: "/dev/full"})
	if err := w.Write(map[string]string{"key": "value"}); err == nil {
		t.Error("expected file write error, got nil")
	}
}

func TestPathResolverSuccess(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get working directory")
	}
	pr := newResolver()
	expected := filepath.Join(filepath.Dir(wd), "src", "settings", "server", "conf.json")
	resolved, err := pr.resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != expected {
		t.Errorf("expected %s, got %s", expected, resolved)
	}
}
