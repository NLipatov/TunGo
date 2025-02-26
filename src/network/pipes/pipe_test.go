package pipes

import (
	"bytes"
	"errors"
	"testing"
)

type errorWriter struct{}

func (ew errorWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write error")
}

func TestDefaultPipe_PassSuccess(t *testing.T) {
	var buf bytes.Buffer
	reader := bytes.NewBufferString("dummy")
	pipe := NewDefaultPipe(reader, &buf)

	data := []byte("test data")
	if err := pipe.Pass(data); err != nil {
		t.Fatalf("Pass returned unexpected error: %v", err)
	}

	if buf.String() != "test data" {
		t.Errorf("Expected 'test data', got '%s'", buf.String())
	}
}

func TestDefaultPipe_PassError(t *testing.T) {
	reader := bytes.NewBufferString("dummy")
	pipe := NewDefaultPipe(reader, errorWriter{})

	data := []byte("test data")
	err := pipe.Pass(data)
	if err == nil {
		t.Fatal("Expected error from Pass, got nil")
	}

	if err.Error() != "write error" {
		t.Errorf("Expected error 'write error', got '%v'", err)
	}
}
