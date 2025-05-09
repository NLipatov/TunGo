// handshake/defaultsessionidentifier_test.go
package handshake

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// mockDefaultSessionIdentifierReader simulates io.Reader for DefaultSessionIdentifier tests.
type mockDefaultSessionIdentifierReader struct {
	data []byte
	err  error
}

func (m *mockDefaultSessionIdentifierReader) Read(p []byte) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	n := copy(p, m.data)
	// if not enough data, simulate EOF after copying
	if len(m.data) > n {
		m.data = m.data[n:]
		return n, nil
	}
	m.data = nil
	return n, io.EOF
}

func TestDefaultSessionIdentifier_SuccessExact32(t *testing.T) {
	// Prepare exactly 32 bytes
	input := make([]byte, 32)
	for i := range input {
		input[i] = byte(i + 1)
	}
	rdr := &mockDefaultSessionIdentifierReader{data: append([]byte(nil), input...)}
	id := NewSessionIdentifier(rdr)

	got, err := id.Identify()
	if err != nil {
		t.Fatalf("Identify() returned unexpected error: %v", err)
	}

	var want [32]byte
	copy(want[:], input)
	if got != want {
		t.Errorf("Identify() = %v, want %v", got, want)
	}
}

func TestDefaultSessionIdentifier_SuccessMoreThan32(t *testing.T) {
	// Provide more than 32 bytes; only first 32 should be used
	full := make([]byte, 40)
	for i := range full {
		full[i] = byte(i + 10)
	}
	rdr := &mockDefaultSessionIdentifierReader{data: append([]byte(nil), full...)}
	id := NewSessionIdentifier(rdr)

	got, err := id.Identify()
	if err != nil {
		t.Fatalf("Identify() with extra data error: %v", err)
	}

	var want [32]byte
	copy(want[:], full[:32])
	if got != want {
		t.Errorf("Identify() with extra data = %v, want %v", got, want)
	}
}

func TestDefaultSessionIdentifier_ErrorShortRead(t *testing.T) {
	// Provide fewer than 32 bytes and no error until EOF
	short := make([]byte, 10)
	rdr := &mockDefaultSessionIdentifierReader{data: short}
	id := NewSessionIdentifier(rdr)

	_, err := id.Identify()
	if err == nil {
		t.Fatal("Identify() expected error on short read, got nil")
	}
	// ensure wrapped error message
	if !strings.Contains(err.Error(), "failed to derive session ID") {
		t.Errorf("error message = %q, want contain %q", err.Error(), "failed to derive session ID")
	}
}

func TestDefaultSessionIdentifier_ErrorReaderErr(t *testing.T) {
	// Reader returns explicit error
	expErr := fmt.Errorf("read failure")
	rdr := &mockDefaultSessionIdentifierReader{err: expErr}
	id := NewSessionIdentifier(rdr)

	_, err := id.Identify()
	if err == nil {
		t.Fatal("Identify() expected reader error, got nil")
	}
	if !errors.Is(err, expErr) {
		t.Errorf("Identify() error = %v, want wrapped %v", err, expErr)
	}
}
