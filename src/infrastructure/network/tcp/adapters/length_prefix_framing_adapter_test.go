package adapters

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"

	framelimit "tungo/domain/network/ip/frame_limit"
)

// --- Test helpers / mock ---

// LengthPrefixFramingAdapterMockConn is a controllable mock for application.Transport.
// It supports partial reads/writes, injected errors, and captures written bytes.
type LengthPrefixFramingAdapterMockConn struct {
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

func (m *LengthPrefixFramingAdapterMockConn) Read(p []byte) (int, error) {
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

func (m *LengthPrefixFramingAdapterMockConn) Write(p []byte) (int, error) {
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

func (m *LengthPrefixFramingAdapterMockConn) Close() error { return m.closeErr }

func mkFrame(payload []byte) []byte {
	b := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(b[:2], uint16(len(payload)))
	copy(b[2:], payload)
	return b
}

// --- Constructor tests ---

func TestNewLengthPrefixFramingAdapter_ErrNilAdapter(t *testing.T) {
	capv, _ := framelimit.NewCap(10)
	if _, err := NewLengthPrefixFramingAdapter(nil, capv); err == nil {
		t.Fatal("expected error for nil adapter")
	}
}

func TestNewLengthPrefixFramingAdapter_ErrNonPositiveCap(t *testing.T) {
	// Bypass domain constructor intentionally to hit adapter check.
	if _, err := NewLengthPrefixFramingAdapter(&LengthPrefixFramingAdapterMockConn{}, framelimit.Cap(0)); err == nil {
		t.Fatal("expected error for non-positive cap")
	}
	if _, err := NewLengthPrefixFramingAdapter(&LengthPrefixFramingAdapterMockConn{}, framelimit.Cap(-1)); err == nil {
		t.Fatal("expected error for negative cap")
	}
}

func TestNewLengthPrefixFramingAdapter_ErrCapExceedsU16(t *testing.T) {
	if _, err := NewLengthPrefixFramingAdapter(&LengthPrefixFramingAdapterMockConn{}, framelimit.Cap(math.MaxUint16+1)); err == nil {
		t.Fatal("expected error for cap > u16")
	}
}

func TestNewLengthPrefixFramingAdapter_OK(t *testing.T) {
	capv, _ := framelimit.NewCap(1024)
	a, err := NewLengthPrefixFramingAdapter(&LengthPrefixFramingAdapterMockConn{}, capv)
	if err != nil || a == nil {
		t.Fatalf("unexpected constructor result: a=%v err=%v", a, err)
	}
}

// --- Write tests ---

func TestWrite_Success_WithPartialPrefixAndPayload(t *testing.T) {
	payload := []byte("hello-world")
	mock := &LengthPrefixFramingAdapterMockConn{
		// prefix: 1 + 1; payload: 2 + rest
		writeChunks: []int{1, 1, 2, len(payload) - 2},
	}
	capv, _ := framelimit.NewCap(65535)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

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

func TestWrite_ZeroLengthFrame(t *testing.T) {
	mock := &LengthPrefixFramingAdapterMockConn{}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Write(nil); !errors.Is(err, ErrZeroLengthFrame) {
		t.Fatalf("expected ErrZeroLengthFrame, got %v", err)
	}
}

func TestWrite_ExceedsDomainCap(t *testing.T) {
	mock := &LengthPrefixFramingAdapterMockConn{}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Write(make([]byte, 11)); !errors.Is(err, framelimit.ErrCapExceeded) {
		t.Fatalf("expected framelimit.ErrCapExceeded, got %v", err)
	}
}

func TestWrite_ExceedsU16ByPayloadLen(t *testing.T) {
	// cap == u16, but payload > u16 -> should fail domain cap check before writing header
	mock := &LengthPrefixFramingAdapterMockConn{}
	capv, _ := framelimit.NewCap(math.MaxUint16)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Write(make([]byte, math.MaxUint16+1)); !errors.Is(err, framelimit.ErrCapExceeded) {
		t.Fatalf("expected framelimit.ErrCapExceeded, got %v", err)
	}
}

func TestWrite_PrefixShortWriteZero(t *testing.T) {
	payload := []byte("abc")
	mock := &LengthPrefixFramingAdapterMockConn{
		writeChunks: []int{0}, // first Write returns (0, nil) -> io.ErrShortWrite
	}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Write(payload); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
}

func TestWrite_PrefixWriteError(t *testing.T) {
	payload := []byte("abc")
	mock := &LengthPrefixFramingAdapterMockConn{
		writeErrAt: 1,
		writeErr:   io.ErrClosedPipe,
	}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Write(payload); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestWrite_PayloadWriteErrorAfterSomeBytes(t *testing.T) {
	// Writer returns (n>0, err!=nil) — writeFull must still return the error.
	payload := []byte("abcdef")
	mock := &LengthPrefixFramingAdapterMockConn{
		writeChunks: []int{2, 1}, // header, then write 1 byte of payload
		writeErrAt:  3,
		writeErr:    io.ErrClosedPipe,
	}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Write(payload); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

// --- Read tests ---

func TestRead_Success_WithPartials(t *testing.T) {
	payload := []byte("read-ok-payload")
	frame := mkFrame(payload)
	mock := &LengthPrefixFramingAdapterMockConn{
		readData:   frame,
		readChunks: []int{1, 1, 3, 2, len(payload) - 5}, // split hdr+payload into several reads
	}
	capv, _ := framelimit.NewCap(1024)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

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
	mock := &LengthPrefixFramingAdapterMockConn{
		readData:   []byte{0x00},
		readChunks: []int{1},
	}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	_, err := a.Read(make([]byte, 10))
	if err == nil || !errors.Is(err, ErrInvalidLengthPrefixHeader) {
		t.Fatalf("expected ErrInvalidLengthPrefixHeader, got %v", err)
	}
}

func TestRead_ZeroLengthFrame(t *testing.T) {
	mock := &LengthPrefixFramingAdapterMockConn{readData: []byte{0x00, 0x00}}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Read(make([]byte, 1)); !errors.Is(err, ErrZeroLengthFrame) {
		t.Fatalf("expected ErrZeroLengthFrame, got %v", err)
	}
}

func TestRead_ExceedsDomainCap_NoDrain(t *testing.T) {
	// header says 3 bytes, but domain cap is 2 -> expect domain error, payload remains unread.
	frame := mkFrame([]byte("xyz")) // len=3
	mock := &LengthPrefixFramingAdapterMockConn{readData: frame}
	capv, _ := framelimit.NewCap(2)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Read(make([]byte, 3)); !errors.Is(err, framelimit.ErrCapExceeded) {
		t.Fatalf("expected framelimit.ErrCapExceeded, got %v", err)
	}
	// No drain: next byte to read should be the first payload byte, breaking alignment for any next Read.
	// We won't call a.Read again (per contract caller must close), but ensure mock still has unread data:
	if rem := len(mock.readData) - mock.readOff; rem == 0 {
		t.Fatalf("expected unread payload to remain, but none left")
	}
}

func TestRead_ShortBuffer_NoDrain(t *testing.T) {
	payload := []byte("some-long-payload")
	frame := mkFrame(payload)
	mock := &LengthPrefixFramingAdapterMockConn{readData: frame}
	capv, _ := framelimit.NewCap(1024)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Read(make([]byte, 4)); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("expected io.ErrShortBuffer, got %v", err)
	}
	// No drain — unread payload should remain.
	if rem := len(mock.readData) - mock.readOff; rem == 0 {
		t.Fatalf("expected unread payload to remain, but none left")
	}
}

func TestRead_PayloadReadError(t *testing.T) {
	// header says 5 bytes, but only 3 available -> io.ReadFull returns error
	hdr := []byte{0x00, 0x05}
	data := append(hdr, []byte("abc")...)
	mock := &LengthPrefixFramingAdapterMockConn{readData: data}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if _, err := a.Read(make([]byte, 5)); err == nil {
		t.Fatal("expected payload read error, got nil")
	}
}

// --- Close tests ---

func TestClose_OK(t *testing.T) {
	mock := &LengthPrefixFramingAdapterMockConn{}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if err := a.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClose_Err(t *testing.T) {
	mock := &LengthPrefixFramingAdapterMockConn{closeErr: io.ErrClosedPipe}
	capv, _ := framelimit.NewCap(10)
	a, _ := NewLengthPrefixFramingAdapter(mock, capv)

	if err := a.Close(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}
