package pipes

import (
	"bytes"
	"errors"
	"testing"
)

type errorReader struct{}

func (er *errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read error")
}

func TestReaderPipe_Success(t *testing.T) {
	expected := "hello, world"
	reader := bytes.NewBufferString(expected)
	output := new(bytes.Buffer)
	basePipe := NewDefaultPipe(nil, output)
	rp := NewReaderPipe(basePipe, reader)

	buf := make([]byte, 1024)
	if err := rp.Pass(buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.String() != expected {
		t.Errorf("expected output %q, got %q", expected, output.String())
	}
}

func TestReaderPipe_ReadError(t *testing.T) {
	errReader := &errorReader{}
	output := new(bytes.Buffer)
	basePipe := NewDefaultPipe(nil, output)
	rp := NewReaderPipe(basePipe, errReader)

	buf := make([]byte, 1024)
	err := rp.Pass(buf)
	if err == nil {
		t.Fatal("expected error from ReaderPipe.Pass, got nil")
	}
	if err.Error() != "read error" {
		t.Errorf("expected error 'read error', got %q", err.Error())
	}
}
