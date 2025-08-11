package adapters

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
	"tungo/infrastructure/network"
)

// AdapterMockConn is a controllable mock for application.ConnectionAdapter.
// It supports partial reads/writes, injected errors, and captures written bytes.
type AdapterMockConn struct {
	// Read side
	readData   []byte
	readOff    int
	readChunks []int // per Read() how many bytes to return (defaults to as many as possible)
	readErrAt  int   // 1-based call index to return readErr
	readErr    error

	// Write side
	writeChunks []int // per Write() how many bytes to accept (defaults to len(p))
	writeErrAt  int   // 1-based call index to return writeErr
	writeErr    error
	writeBuf    bytes.Buffer // captures written bytes

	// Close
	closeErr error

	rCalls int
	wCalls int
}

func (m *AdapterMockConn) Read(p []byte) (int, error) {
	m.rCalls++
	if m.readErrAt > 0 && m.rCalls == m.readErrAt {
		if m.readErr == nil {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, m.readErr
	}
	if m.readOff >= len(m.readData) {
		return 0, io.EOF
	}
	n := len(m.readData) - m.readOff
	if len(m.readChunks) >= m.rCalls {
		want := m.readChunks[m.rCalls-1]
		if want < n {
			n = want
		}
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, m.readData[m.readOff:m.readOff+n])
	m.readOff += n
	return n, nil
}

func (m *AdapterMockConn) Write(p []byte) (int, error) {
	m.wCalls++
	if m.writeErrAt > 0 && m.wCalls == m.writeErrAt {
		if m.writeErr == nil {
			return 0, io.ErrClosedPipe
		}
		return 0, m.writeErr
	}
	n := len(p)
	if len(m.writeChunks) >= m.wCalls {
		want := m.writeChunks[m.wCalls-1]
		if want < n {
			n = want
		}
	}
	if n > 0 {
		m.writeBuf.Write(p[:n])
	}
	return n, nil
}

func (m *AdapterMockConn) Close() error { return m.closeErr }

func mkFrame(payload []byte) []byte {
	b := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(b[:2], uint16(len(payload)))
	copy(b[2:], payload)
	return b
}

// ---------------- Write tests ----------------

func TestAdapter_Write_Success_PartialPrefixAndPayload(t *testing.T) {
	payload := []byte("hello-world")
	mock := &AdapterMockConn{
		// prefix: 1 + 1; payload: 2 + rest
		writeChunks: []int{1, 1, 2, len(payload) - 2},
	}
	a := NewTcpAdapter(mock)

	n, err := a.Write(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("n=%d want=%d", n, len(payload))
	}

	want := mkFrame(payload)
	got := mock.writeBuf.Bytes()
	if !bytes.Equal(want, got) {
		t.Fatalf("written mismatch:\nwant=%x\ngot =%x", want, got)
	}
}

func TestAdapter_Write_PrefixShortWriteZero(t *testing.T) {
	payload := []byte("abc")
	mock := &AdapterMockConn{
		writeChunks: []int{0}, // first Write returns (0, nil)
	}
	a := NewTcpAdapter(mock)

	_, err := a.Write(payload)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("expected ErrShortWrite, got %v", err)
	}
}

func TestAdapter_Write_PrefixWriteError(t *testing.T) {
	payload := []byte("abc")
	mock := &AdapterMockConn{
		writeErrAt: 1,
		writeErr:   io.ErrClosedPipe,
	}
	a := NewTcpAdapter(mock)

	_, err := a.Write(payload)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestAdapter_Write_PayloadWriteError(t *testing.T) {
	payload := []byte("abcdef")
	mock := &AdapterMockConn{
		writeChunks: []int{2}, // prefix fully
		writeErrAt:  2,        // first payload write errors
		writeErr:    io.ErrClosedPipe,
	}
	a := NewTcpAdapter(mock)

	_, err := a.Write(payload)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestAdapter_Write_PayloadWriteSomeAndError(t *testing.T) {
	// Writer returns (n>0, err!=nil) — writeFull must still return the error.
	payload := []byte("abcdef")
	mock := &AdapterMockConn{
		writeChunks: []int{2, 1}, // prefix, then write 1 byte of payload
		writeErrAt:  3,
		writeErr:    io.ErrClosedPipe,
	}
	a := NewTcpAdapter(mock)

	_, err := a.Write(payload)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestAdapter_Write_TooLarge_U16First(t *testing.T) {
	// u16 bound is checked first; always reachable
	payload := make([]byte, math.MaxUint16+1)
	mock := &AdapterMockConn{}
	a := NewTcpAdapter(mock)

	if _, err := a.Write(payload); err == nil {
		t.Fatal("expected error for u16 bound")
	}
}

func TestAdapter_Write_TooLarge_Protocol(t *testing.T) {
	payload := make([]byte, network.MaxPacketLengthBytes+1)
	mock := &AdapterMockConn{}
	a := NewTcpAdapter(mock)

	if _, err := a.Write(payload); err == nil {
		t.Fatal("expected error for protocol limit")
	}
}

// ---------------- Read tests ----------------

func TestAdapter_Read_Success_WithPartials(t *testing.T) {
	payload := []byte("read-ok-payload")
	frame := mkFrame(payload)
	mock := &AdapterMockConn{
		readData:   frame,
		readChunks: []int{1, 1, 3, 2, len(payload) - 5}, // split hdr+payload
	}
	a := NewTcpAdapter(mock)

	buf := make([]byte, len(payload))
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("n=%d want=%d", n, len(payload))
	}
	if !bytes.Equal(buf[:n], payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestAdapter_Read_PrefixEOF(t *testing.T) {
	mock := &AdapterMockConn{
		readData:   []byte{0x00}, // incomplete header (need 2 bytes)
		readChunks: []int{1},
	}
	a := NewTcpAdapter(mock)

	_, err := a.Read(make([]byte, 10))
	if err == nil {
		t.Fatal("expected error on incomplete prefix")
	}
}

func TestAdapter_Read_ZeroLength(t *testing.T) {
	mock := &AdapterMockConn{readData: []byte{0x00, 0x00}}
	a := NewTcpAdapter(mock)

	if _, err := a.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected error for zero-length frame")
	}
}

func TestAdapter_Read_TooLargeProtocol(t *testing.T) {
	t.Skip("protocol limit >= u16 max; cannot exceed on read path")
	ln := network.MaxPacketLengthBytes + 1
	hdr := []byte{byte(ln >> 8), byte(ln)}
	mock := &AdapterMockConn{readData: hdr}
	a := NewTcpAdapter(mock)

	if _, err := a.Read(make([]byte, ln)); err == nil {
		t.Fatal("expected error for frame length > protocol limit")
	}
}

func TestAdapter_Read_ShortBuffer_DrainsPayload(t *testing.T) {
	payload := []byte("some-long-payload")
	frame := mkFrame(payload)
	mock := &AdapterMockConn{readData: frame}
	a := NewTcpAdapter(mock)

	// small buffer triggers ErrShortBuffer; implementation drains payload
	n, err := a.Read(make([]byte, 4))
	if !errors.Is(err, io.ErrShortBuffer) || n != 0 {
		t.Fatalf("expected ErrShortBuffer with n=0, got n=%d err=%v", n, err)
	}

	// After drain, stream is aligned; next read should hit EOF on prefix
	_, err = a.Read(make([]byte, 10))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after drain, got %v", err)
	}
}

func TestAdapter_Read_PayloadEOF(t *testing.T) {
	// header says 5 bytes, only 3 available
	hdr := []byte{0x00, 0x05}
	data := append(hdr, []byte("abc")...)
	mock := &AdapterMockConn{readData: data}
	a := NewTcpAdapter(mock)

	_, err := a.Read(make([]byte, 5))
	if err == nil {
		t.Fatal("expected error on incomplete payload")
	}
}

// ---------------- drainN tests ----------------

// Test drainN drains big amount (> chunk) and returns nil.
func TestAdapter_drainN_OK(t *testing.T) {
	// Build a reader with 5000 bytes to exercise both branches inside drainN
	big := make([]byte, 5000)
	r := bytes.NewReader(big)
	a := NewTcpAdapter(&AdapterMockConn{}).(*Adapter)
	if err := a.drainN(r, len(big)); err != nil {
		t.Fatalf("drainN failed: %v", err)
	}
}

// Test drainN error (not enough bytes to drain).
func TestAdapter_drainN_Err(t *testing.T) {
	small := make([]byte, 1000)
	r := bytes.NewReader(small)
	a := NewTcpAdapter(&AdapterMockConn{}).(*Adapter)
	if err := a.drainN(r, 2000); !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF-ish from drainN, got %v", err)
	}
}

// ---------------- Close tests ----------------

func TestAdapter_Close_OK(t *testing.T) {
	mock := &AdapterMockConn{}
	a := NewTcpAdapter(mock)
	if err := a.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapter_Close_Err(t *testing.T) {
	mock := &AdapterMockConn{closeErr: io.ErrClosedPipe}
	a := NewTcpAdapter(mock)
	if err := a.Close(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}
