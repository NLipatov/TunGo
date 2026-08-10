package tcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

// --- Test helpers / mock ---

// framedConnMockConn is a controllable mock for config.Transport.
// It supports partial reads/writes, injected errors, and captures written bytes.
type framedConnMockConn struct {
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

type framedConnDiscard struct{}

func (framedConnDiscard) Read([]byte) (int, error)    { return 0, io.EOF }
func (framedConnDiscard) Write(p []byte) (int, error) { return len(p), nil }
func (framedConnDiscard) Close() error                { return nil }

func (m *framedConnMockConn) Read(p []byte) (int, error) {
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

func (m *framedConnMockConn) Write(p []byte) (int, error) {
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
		_, _ = m.writeBuf.Write(p[:n])
	}
	return n, nil
}

func (m *framedConnMockConn) Close() error { return m.closeErr }

func mkFrame(payload []byte) []byte {
	b := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(b[:2], uint16(len(payload)))
	copy(b[2:], payload)
	return b
}

// --- Constructor tests ---

func TestNewFramedConn_ErrNilAdapter(t *testing.T) {
	capv := 10
	if _, err := NewFramedConn(nil, capv); err == nil {
		t.Fatal("expected error for nil adapter")
	}
}

func TestNewFramedConn_ErrNonPositiveCap(t *testing.T) {
	// The adapter owns frame-cap validation.
	if _, err := NewFramedConn(&framedConnMockConn{}, 0); err == nil {
		t.Fatal("expected error for non-positive cap")
	}
	if _, err := NewFramedConn(&framedConnMockConn{}, -1); err == nil {
		t.Fatal("expected error for negative cap")
	}
}

func TestNewFramedConn_ErrCapExceedsU16(t *testing.T) {
	if _, err := NewFramedConn(&framedConnMockConn{}, math.MaxUint16+1); err == nil {
		t.Fatal("expected error for cap > u16")
	}
}

func TestNewFramedConn_OK(t *testing.T) {
	capv := 1024
	a, err := NewFramedConn(&framedConnMockConn{}, capv)
	if err != nil || a == nil {
		t.Fatalf("unexpected constructor result: a=%v err=%v", a, err)
	}
}

// --- Write tests ---

func TestWrite_Success_WithPartialPrefixAndPayload(t *testing.T) {
	payload := []byte("hello-world")
	mock := &framedConnMockConn{
		// prefix: 1 + 1; payload: 2 + rest
		writeChunks: []int{1, 1, 2, len(payload) - 2},
	}
	capv := 65535
	a, _ := NewFramedConn(mock, capv)

	n, err := a.Write(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("n=%d want=%d", n, len(payload))
	}
	want := mkFrame(payload)
	if got := mock.writeBuf.Bytes(); !bytes.Equal(want, got) {
		t.Fatalf("written mismatch:\nwant=%x\ngot =%x", want, got)
	}
}

func TestWrite_DoesNotAllocate(t *testing.T) {
	a, err := NewFramedConn(framedConnDiscard{}, 1500)
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 1500)

	var n int
	allocs := testing.AllocsPerRun(100, func() {
		n, err = a.Write(payload)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != len(payload) {
		t.Fatalf("n=%d want=%d", n, len(payload))
	}
	if allocs != 0 {
		t.Fatalf("Write allocations=%v, want 0", allocs)
	}
}

func TestWrite_ZeroLengthFrame(t *testing.T) {
	mock := &framedConnMockConn{}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Write(nil); !errors.Is(err, ErrZeroLengthFrame) {
		t.Fatalf("expected ErrZeroLengthFrame, got %v", err)
	}
}

func TestWrite_ExceedsFrameCap(t *testing.T) {
	mock := &framedConnMockConn{}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Write(make([]byte, 11)); !errors.Is(err, ErrFrameCapExceeded) {
		t.Fatalf("expected ErrFrameCapExceeded, got %v", err)
	}
}

func TestWrite_ExceedsU16ByPayloadLen(t *testing.T) {
	// cap == u16, but payload > u16 -> should fail frame-cap check before writing header
	mock := &framedConnMockConn{}
	capv := math.MaxUint16
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Write(make([]byte, math.MaxUint16+1)); !errors.Is(err, ErrFrameCapExceeded) {
		t.Fatalf("expected ErrFrameCapExceeded, got %v", err)
	}
}

func TestWrite_PrefixShortWriteZero(t *testing.T) {
	payload := []byte("abc")
	mock := &framedConnMockConn{
		writeChunks: []int{0}, // first Write returns (0, nil) -> io.ErrShortWrite
	}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Write(payload); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
}

func TestWrite_PrefixWriteError(t *testing.T) {
	payload := []byte("abc")
	mock := &framedConnMockConn{
		writeErrAt: 1,
		writeErr:   io.ErrClosedPipe,
	}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Write(payload); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestWrite_PayloadWriteErrorAfterSomeBytes(t *testing.T) {
	// Writer returns (n>0, err!=nil) — writeFull must still return the error.
	payload := []byte("abcdef")
	mock := &framedConnMockConn{
		writeChunks: []int{2, 1}, // header, then write 1 byte of payload
		writeErrAt:  3,
		writeErr:    io.ErrClosedPipe,
	}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Write(payload); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

// --- Read tests ---

func TestRead_Success_WithPartials(t *testing.T) {
	payload := []byte("read-ok-payload")
	frame := mkFrame(payload)
	mock := &framedConnMockConn{
		readData:   frame,
		readChunks: []int{1, 1, 3, 2, len(payload) - 5}, // split hdr+payload into several reads
	}
	capv := 1024
	a, _ := NewFramedConn(mock, capv)

	buf := make([]byte, len(payload))
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("n=%d want=%d", n, len(payload))
	}
	if got := string(buf[:n]); got != string(payload) {
		t.Fatalf("payload mismatch: got=%q want=%q", got, string(payload))
	}
}

func TestRead_PrefixError_Wrapped(t *testing.T) {
	// Only 1 byte of header -> io.ErrUnexpectedEOF; must be wrapped into ErrInvalidLengthPrefixHeader.
	mock := &framedConnMockConn{
		readData:   []byte{0x00},
		readChunks: []int{1},
	}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	_, err := a.Read(make([]byte, 10))
	if err == nil || !errors.Is(err, ErrInvalidLengthPrefixHeader) {
		t.Fatalf("expected ErrInvalidLengthPrefixHeader, got %v", err)
	}
}

func TestRead_ZeroLengthFrame(t *testing.T) {
	mock := &framedConnMockConn{readData: []byte{0x00, 0x00}}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Read(make([]byte, 1)); !errors.Is(err, ErrZeroLengthFrame) {
		t.Fatalf("expected ErrZeroLengthFrame, got %v", err)
	}
}

func TestRead_ExceedsFrameCap_NoDrain(t *testing.T) {
	// Header says 3 bytes, but frame cap is 2; payload must remain unread.
	frame := mkFrame([]byte("xyz")) // len=3
	mock := &framedConnMockConn{readData: frame}
	capv := 2
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Read(make([]byte, 3)); !errors.Is(err, ErrFrameCapExceeded) {
		t.Fatalf("expected ErrFrameCapExceeded, got %v", err)
	}
	// Contract: adapter does NOT drain payload on error; caller must close.
	// (With buffering, mock data may be consumed into bufReader — that's fine,
	// the payload is still not returned to the caller.)
}

func TestRead_ShortBuffer_NoDrain(t *testing.T) {
	payload := []byte("some-long-payload")
	frame := mkFrame(payload)
	mock := &framedConnMockConn{readData: frame}
	capv := 1024
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Read(make([]byte, 4)); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("expected io.ErrShortBuffer, got %v", err)
	}
	// Contract: adapter does NOT drain payload on error; caller must close.
}

func TestRead_PayloadReadError(t *testing.T) {
	// header says 5 bytes, but only 3 available -> io.ReadFull returns error
	hdr := []byte{0x00, 0x05}
	data := append(hdr, []byte("abc")...)
	mock := &framedConnMockConn{readData: data}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if _, err := a.Read(make([]byte, 5)); err == nil {
		t.Fatal("expected payload read error, got nil")
	}
}

// --- Close tests ---

func TestClose_OK(t *testing.T) {
	mock := &framedConnMockConn{}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if err := a.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClose_Err(t *testing.T) {
	mock := &framedConnMockConn{closeErr: io.ErrClosedPipe}
	capv := 10
	a, _ := NewFramedConn(mock, capv)

	if err := a.Close(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

// --- Buffering tests ---

func TestRead_BufferedReducesSyscalls(t *testing.T) {
	// Two complete frames in mock — no readChunks, so mock returns all data at once.
	frame1 := mkFrame([]byte("frame-one"))
	frame2 := mkFrame([]byte("frame-two"))
	mock := &framedConnMockConn{
		readData: append(frame1, frame2...),
	}
	capv := 1024
	a, _ := NewFramedConn(mock, capv)

	buf := make([]byte, 1024)
	n1, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read frame 1: %v", err)
	}
	if got := string(buf[:n1]); got != "frame-one" {
		t.Fatalf("frame 1 payload: got=%q want=%q", got, "frame-one")
	}

	n2, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read frame 2: %v", err)
	}
	if got := string(buf[:n2]); got != "frame-two" {
		t.Fatalf("frame 2 payload: got=%q want=%q", got, "frame-two")
	}

	// Both frames served from a single underlying Read thanks to bufio.Reader.
	if mock.rCalls != 1 {
		t.Fatalf("expected 1 underlying Read call, got %d", mock.rCalls)
	}
}
