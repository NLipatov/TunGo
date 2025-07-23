package framing

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

// mockConn is a mock implementation of application.ConnectionAdapter.
type mockConn struct {
	readBuf   *bytes.Buffer
	writeBuf  *bytes.Buffer
	closeErr  error
	writeErrs []error // errors for each Write call
	writeN    int     // current Write call index
	readErr   error
	closed    bool
}

func (m *mockConn) Write(p []byte) (int, error) {
	if m.writeErrs != nil && m.writeN < len(m.writeErrs) && m.writeErrs[m.writeN] != nil {
		m.writeN++
		return 0, m.writeErrs[m.writeN-1]
	}
	m.writeN++
	return m.writeBuf.Write(p)
}

func (m *mockConn) Read(p []byte) (int, error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	return m.readBuf.Read(p)
}

func (m *mockConn) Close() error {
	m.closed = true
	return m.closeErr
}

// Test Write and Read (happy path)
func TestTCPFramingAdapter_WriteAndRead(t *testing.T) {
	// Prepare mock connection with a buffer
	conn := &mockConn{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
	adapter := NewTCPFramingAdapter(conn)

	payload := []byte("hello world")

	// Write should write length + payload
	n, err := adapter.Write(payload)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write: expected %d bytes, got %d", len(payload), n)
	}
	// Copy written data to readBuf for reading
	conn.readBuf.Write(conn.writeBuf.Bytes())

	// Read should read payload
	buf := make([]byte, 128)
	readN, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if readN != len(payload) {
		t.Fatalf("Read: expected %d bytes, got %d", len(payload), readN)
	}
	if string(buf[:readN]) != string(payload) {
		t.Fatalf("Read: expected payload %q, got %q", payload, buf[:readN])
	}
}

// Test Write error on length prefix
func TestTCPFramingAdapter_Write_LengthPrefixError(t *testing.T) {
	conn := &mockConn{
		writeBuf:  bytes.NewBuffer(nil),
		writeErrs: []error{errors.New("length write error")},
	}
	adapter := NewTCPFramingAdapter(conn)
	_, err := adapter.Write([]byte("bar"))
	if err == nil || err.Error() != "length write error" {
		t.Fatalf("expected length write error, got: %v", err)
	}
}

// Test Write error on payload
func TestTCPFramingAdapter_Write_PayloadError(t *testing.T) {
	conn := &mockConn{
		writeBuf:  bytes.NewBuffer(nil),
		writeErrs: []error{nil, errors.New("payload write error")}, // 2nd Write fails
	}
	adapter := NewTCPFramingAdapter(conn)
	_, err := adapter.Write([]byte("baz"))
	if err == nil || err.Error() != "payload write error" {
		t.Fatalf("expected payload write error, got: %v", err)
	}
}

// Test Read error on length prefix
func TestTCPFramingAdapter_Read_LengthPrefixError(t *testing.T) {
	conn := &mockConn{
		readBuf: bytes.NewBuffer(nil), // empty buffer
	}
	adapter := NewTCPFramingAdapter(conn)
	buf := make([]byte, 10)
	_, err := adapter.Read(buf)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "failed to read length prefix") {
		t.Fatalf("expected io.EOF or prefix error, got: %v", err)
	}
}

// Test Read error on payload
func TestTCPFramingAdapter_Read_PayloadError(t *testing.T) {
	// Prepare buffer: 4-byte length (payload size=5), only 3 bytes in payload (simulate short read)
	b := &bytes.Buffer{}
	_ = binary.Write(b, binary.BigEndian, uint32(5))
	b.Write([]byte("abc")) // only 3/5 bytes
	conn := &mockConn{
		readBuf: b,
	}
	adapter := NewTCPFramingAdapter(conn)
	buf := make([]byte, 10)
	_, err := adapter.Read(buf)
	if err == nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF for payload, got: %v", err)
	}
}

// Test Read returns io.ErrShortBuffer if buffer is too small
func TestTCPFramingAdapter_Read_ShortBuffer(t *testing.T) {
	// Prepare buffer: 4-byte length (payload size=9), 9 bytes of payload
	b := &bytes.Buffer{}
	_ = binary.Write(b, binary.BigEndian, uint32(9))
	b.Write([]byte("123456789"))
	conn := &mockConn{
		readBuf: b,
	}
	adapter := NewTCPFramingAdapter(conn)
	buf := make([]byte, 5) // too small
	_, err := adapter.Read(buf)
	if err != io.ErrShortBuffer {
		t.Fatalf("expected io.ErrShortBuffer, got: %v", err)
	}
}

// Test Close (no error)
func TestTCPFramingAdapter_Close_OK(t *testing.T) {
	conn := &mockConn{}
	adapter := NewTCPFramingAdapter(conn)
	err := adapter.Close()
	if err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
	if !conn.closed {
		t.Fatalf("Close did not set closed flag")
	}
}

// Test Close (with error)
func TestTCPFramingAdapter_Close_Error(t *testing.T) {
	conn := &mockConn{closeErr: errors.New("close failed")}
	adapter := NewTCPFramingAdapter(conn)
	err := adapter.Close()
	if err == nil || err.Error() != "close failed" {
		t.Fatalf("expected close error, got: %v", err)
	}
}
