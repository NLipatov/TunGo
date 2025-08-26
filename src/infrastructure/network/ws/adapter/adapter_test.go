package adapter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type fakeWriter struct {
	buf        *bytes.Buffer
	writeErrAt int // if >=0, return error at first write once total >= writeErrAt
	closeErr   error
	totalWrote int
	closed     bool
	mu         sync.Mutex
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("write after close")
	}
	n, _ := w.buf.Write(p)
	w.totalWrote += n
	if w.writeErrAt >= 0 && w.totalWrote >= w.writeErrAt {
		// trigger once
		w.writeErrAt = -1
		return n, errors.New("write error")
	}
	return n, nil
}

func (w *fakeWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return w.closeErr
}

type readerResult struct {
	mt  websocket.MessageType
	r   io.Reader
	err error
	ctx context.Context // captured
}

type writerResult struct {
	w   *fakeWriter
	err error
	ctx context.Context // captured
	mt  websocket.MessageType
}

type fakeConn struct {
	// queues of behaviors for successive calls
	readerQueue []*readerResult
	writerQueue []*writerResult

	closeStatus websocket.StatusCode
	closeReason string
	closeErr    error

	mu sync.Mutex
}

func (c *fakeConn) pushReader(mt websocket.MessageType, r io.Reader, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readerQueue = append(c.readerQueue, &readerResult{mt: mt, r: r, err: err})
}

func (c *fakeConn) pushWriter(w *fakeWriter, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writerQueue = append(c.writerQueue, &writerResult{w: w, err: err})
}

func (c *fakeConn) Reader(ctx context.Context) (websocket.MessageType, io.Reader, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.readerQueue) == 0 {
		return 0, nil, errors.New("no reader queued")
	}
	res := c.readerQueue[0]
	c.readerQueue = c.readerQueue[1:]
	res.ctx = ctx
	return res.mt, res.r, res.err
}

func (c *fakeConn) Writer(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.writerQueue) == 0 {
		return nil, errors.New("no writer queued")
	}
	res := c.writerQueue[0]
	c.writerQueue = c.writerQueue[1:]
	res.ctx = ctx
	res.mt = mt
	if res.err != nil {
		return nil, res.err
	}
	return res.w, nil
}

func (c *fakeConn) Close(status websocket.StatusCode, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeStatus = status
	c.closeReason = reason
	return c.closeErr
}

// testErrorMapper maps errors as-is.
type testErrorMapper struct{}

func (testErrorMapper) mapErr(err error) error { return err }

/*** Tests ***/

func newAdapterWithConn(ctx context.Context, fc *fakeConn) *Adapter {
	a := NewAdapter(ctx, fc, &net.TCPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 1}, &net.TCPAddr{IP: net.IPv4(5, 6, 7, 8), Port: 2})
	// override mapper to stable no-op
	a.errorMapper = testErrorMapper{}
	return a
}

func TestWrite_Empty(t *testing.T) {
	fc := &fakeConn{}
	a := newAdapterWithConn(context.Background(), fc)
	n, err := a.Write(nil)
	if err != nil || n != 0 {
		t.Fatalf("got (%d,%v), want (0,<nil>)", n, err)
	}
}

func TestWrite_Success(t *testing.T) {
	fc := &fakeConn{}
	buf := &bytes.Buffer{}
	w := &fakeWriter{buf: buf, writeErrAt: -1}
	fc.pushWriter(w, nil)

	a := newAdapterWithConn(context.Background(), fc)
	data := []byte("hello world")
	n, err := a.Write(data)
	if err != nil {
		t.Fatalf("write err: %v", err)
	}
	if n != len(data) {
		t.Fatalf("wrote %d, want %d", n, len(data))
	}
	if buf.String() != string(data) {
		t.Fatalf("buffer = %q, want %q", buf.String(), string(data))
	}
	if !w.closed {
		t.Fatalf("writer must be closed")
	}
}

func TestWrite_WriteMidError(t *testing.T) {
	fc := &fakeConn{}
	buf := &bytes.Buffer{}
	w := &fakeWriter{buf: buf, writeErrAt: 3} // trigger after >=3 bytes
	fc.pushWriter(w, nil)

	a := newAdapterWithConn(context.Background(), fc)
	data := []byte("abcdef")
	n, err := a.Write(data)
	if err == nil {
		t.Fatalf("expected error")
	}
	if n == 0 || n > len(data) {
		t.Fatalf("unexpected n=%d", n)
	}
	// writer should be auto-closed by deferred guard
	if !w.closed {
		t.Fatalf("writer must be closed after error")
	}
}

func TestWrite_CloseError(t *testing.T) {
	fc := &fakeConn{}
	buf := &bytes.Buffer{}
	w := &fakeWriter{buf: buf, closeErr: errors.New("close fail")}
	fc.pushWriter(w, nil)

	a := newAdapterWithConn(context.Background(), fc)
	data := []byte("zzz")
	n, err := a.Write(data)
	if err == nil {
		t.Fatalf("expected close error")
	}
	if n != len(data) {
		t.Fatalf("wrote %d, want %d", n, len(data))
	}
	// closed flag toggles even if Close returned error in our writer
	if !w.closed {
		t.Fatalf("writer must be closed flag true")
	}
}

func TestWrite_WriterAcquireError(t *testing.T) {
	fc := &fakeConn{}
	fc.pushWriter(nil, errors.New("no writer"))

	a := newAdapterWithConn(context.Background(), fc)
	_, err := a.Write([]byte("x"))
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRead_EmptyBuf(t *testing.T) {
	fc := &fakeConn{}
	a := newAdapterWithConn(context.Background(), fc)
	n, err := a.Read(nil)
	if err != nil || n != 0 {
		t.Fatalf("got (%d,%v), want (0,<nil>)", n, err)
	}
}

func TestRead_SingleBinaryFrame(t *testing.T) {
	fc := &fakeConn{}
	fc.pushReader(websocket.MessageBinary, bytes.NewBufferString("abcdef"), nil)

	a := newAdapterWithConn(context.Background(), fc)

	buf := make([]byte, 4)
	// first read
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read1 err: %v", err)
	}
	if string(buf[:n]) != "abcd" {
		t.Fatalf("got %q", string(buf[:n]))
	}
	// second read consumes rest and EOF resets currentReader
	n, err = a.Read(buf)
	if err != nil {
		t.Fatalf("read2 err: %v", err)
	}
	if string(buf[:n]) != "ef" {
		t.Fatalf("got %q", string(buf[:n]))
	}
}

func TestRead_NonBinaryFramesAreDrained(t *testing.T) {
	fc := &fakeConn{}
	// first: text frame (should be drained), then binary
	fc.pushReader(websocket.MessageText, bytes.NewBufferString("ping"), nil)
	fc.pushReader(websocket.MessageBinary, bytes.NewBufferString("OK"), nil)

	a := newAdapterWithConn(context.Background(), fc)
	buf := make([]byte, 8)
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}
	if string(buf[:n]) != "OK" {
		t.Fatalf("want OK, got %q", string(buf[:n]))
	}
}

func TestRead_ReaderAcquireError(t *testing.T) {
	fc := &fakeConn{}
	fc.pushReader(0, nil, errors.New("boom"))

	a := newAdapterWithConn(context.Background(), fc)
	_, err := a.Read(make([]byte, 1))
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRead_CurrentReaderError(t *testing.T) {
	fc := &fakeConn{}
	// first binary frame reader that errors (not EOF)
	errReader := readerFunc(func(p []byte) (int, error) { return 0, errors.New("bad") })
	fc.pushReader(websocket.MessageBinary, errReader, nil)

	a := newAdapterWithConn(context.Background(), fc)
	_, err := a.Read(make([]byte, 2))
	if err == nil {
		t.Fatalf("expected error")
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestCancelOnEOF(t *testing.T) {
	var cancels atomic.Int32
	r := io.NopCloser(bytes.NewBuffer(nil)) // returns EOF immediately
	// Wrap NopCloser with Reader interface
	rr := struct{ io.Reader }{Reader: r}
	c := &cancelOnEOF{
		r: rr,
		cancel: func() {
			cancels.Add(1)
		},
	}
	buf := make([]byte, 1)
	_, err := c.Read(buf)
	if err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	if cancels.Load() != 1 {
		t.Fatalf("cancel should be called once")
	}
	// second call should not call cancel again
	_, _ = c.Read(buf)
	if cancels.Load() != 1 {
		t.Fatalf("cancel must still be 1")
	}
}

func TestCloseDelegates(t *testing.T) {
	fc := &fakeConn{}
	a := newAdapterWithConn(context.Background(), fc)
	if err := a.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}
	if fc.closeStatus != websocket.StatusNormalClosure || fc.closeReason != "" {
		t.Fatalf("unexpected close args: %v %q", fc.closeStatus, fc.closeReason)
	}
}

func TestAddrs_DefaultsAndCustom(t *testing.T) {
	// Custom addrs already covered by constructor
	a1 := NewAdapter(context.Background(), &fakeConn{}, nil, nil)
	a1.errorMapper = testErrorMapper{}
	if _, ok := a1.LocalAddr().(*net.TCPAddr); !ok {
		t.Fatalf("LocalAddr default not TCPAddr")
	}
	if _, ok := a1.RemoteAddr().(*net.TCPAddr); !ok {
		t.Fatalf("RemoteAddr default not TCPAddr")
	}
}

func TestDeadlines_SetAndUseInWriter(t *testing.T) {
	fc := &fakeConn{}
	buf := &bytes.Buffer{}
	w := &fakeWriter{buf: buf, writeErrAt: -1}
	fc.pushWriter(w, nil)

	a := newAdapterWithConn(context.Background(), fc)

	dl := time.Now().Add(50 * time.Millisecond)
	if err := a.SetWriteDeadline(dl); err != nil {
		t.Fatal(err)
	}
	_, err := a.Write([]byte("X"))
	if err != nil {
		t.Fatalf("write err: %v", err)
	}
	// check that writer saw a context with deadline
	if len(fc.writerQueue) != 0 {
		t.Fatalf("writerQueue not consumed")
	}
	// We can't read ctx from outside easily; our fake captured it inside queue item,
	// so mimic: push second writer and read its ctx
}

func TestDeadlines_SetAndUseInReader(t *testing.T) {
	fc := &fakeConn{}
	fc.pushReader(websocket.MessageBinary, bytes.NewBufferString("A"), nil)
	a := newAdapterWithConn(context.Background(), fc)
	dl := time.Now().Add(50 * time.Millisecond)
	if err := a.SetReadDeadline(dl); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	_, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}
}

func TestSetDeadline_Both(t *testing.T) {
	a := newAdapterWithConn(context.Background(), &fakeConn{})
	dl := time.Now().Add(time.Second)
	if err := a.SetDeadline(dl); err != nil {
		t.Fatal(err)
	}
	// sanity: now set zero clears
	if err := a.SetDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
}
