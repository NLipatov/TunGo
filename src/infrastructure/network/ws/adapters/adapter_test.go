package adapters

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
	"tungo/infrastructure/network/ws/adapters/internal"

	"github.com/coder/websocket"
)

/*
   Mocks: follow the rule "mocks are prefixed by the tested structure name".
*/

// AdapterWSConnMock implements WSConn with a scripted sequence of frames/writes.
type AdapterWSConnMock struct {
	mu sync.Mutex

	// Reader script: queue of frames to return.
	readQueue []adapterFrame

	// Writer behavior factory; if nil, default usable writer is returned.
	writerFactory func(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error)

	// Track Close invocations.
	closeCalled bool
	closeCode   websocket.StatusCode
	closeReason string
	closeErr    error
}

type adapterFrame struct {
	mt   websocket.MessageType
	data []byte
	err  error // if non-nil, Reader returns this error (mt/data ignored)
}

func NewAdapterWSConnMock() *AdapterWSConnMock { return &AdapterWSConnMock{} }

func (m *AdapterWSConnMock) EnqueueBinary(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]byte(nil), data...)
	m.readQueue = append(m.readQueue, adapterFrame{mt: websocket.MessageBinary, data: cp})
}

func (m *AdapterWSConnMock) EnqueueText(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]byte(nil), data...)
	m.readQueue = append(m.readQueue, adapterFrame{mt: websocket.MessageText, data: cp})
}

func (m *AdapterWSConnMock) EnqueueErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readQueue = append(m.readQueue, adapterFrame{err: err})
}

func (m *AdapterWSConnMock) Reader(ctx context.Context) (websocket.MessageType, io.Reader, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.readQueue) == 0 {
		// Block until ctx is done to simulate waiting for a frame.
		<-ctx.Done()
		return 0, nil, ctx.Err()
	}

	f := m.readQueue[0]
	m.readQueue = m.readQueue[1:]
	if f.err != nil {
		return 0, nil, f.err
	}
	return f.mt, bytes.NewReader(f.data), nil
}

// AdapterWriteCloserMock simulates a WS writer with controllable behavior.
type AdapterWriteCloserMock struct {
	mu  sync.Mutex
	buf *bytes.Buffer
	// failAfter > 0: write this many bytes, then return error once.
	failAfter       int
	writeErr        error // returned after failAfter is consumed (if set)
	closeErr        error
	blockOnWriteCtx context.Context // if set, block on write until this ctx is done
}

func (w *AdapterWriteCloserMock) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.blockOnWriteCtx != nil {
		<-w.blockOnWriteCtx.Done()
		return 0, w.blockOnWriteCtx.Err()
	}

	if w.failAfter > 0 {
		n := w.failAfter
		if n > len(p) {
			n = len(p)
		}
		_, _ = w.buf.Write(p[:n])
		w.failAfter = 0
		if w.writeErr == nil {
			w.writeErr = errors.New("write-failed")
		}
		return n, w.writeErr
	}
	return w.buf.Write(p)
}

func (w *AdapterWriteCloserMock) Close() error { return w.closeErr }

func (m *AdapterWSConnMock) Writer(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error) {
	if m.writerFactory != nil {
		return m.writerFactory(ctx, typ)
	}
	return &AdapterWriteCloserMock{buf: &bytes.Buffer{}}, nil
}

func (m *AdapterWSConnMock) Close(code websocket.StatusCode, reason string) error {
	m.closeCalled = true
	m.closeCode = code
	m.closeReason = reason
	return m.closeErr
}

// AdapterCopierMock allows controlling/observing drain behavior.
type AdapterCopierMock struct {
	// If blockCtx != nil, Copy blocks until blockCtx is done.
	blockCtx context.Context
	// If err != nil, Copy returns this error.
	err error
	// bytes to report as copied (if > 0). If 0, copy src to dst normally.
	bytesToReport int64
	called        bool
}

func (c *AdapterCopierMock) Copy(dst io.Writer, src io.Reader) (int64, error) {
	c.called = true
	if c.blockCtx != nil {
		<-c.blockCtx.Done()
		return 0, c.blockCtx.Err()
	}
	if c.err != nil {
		return 0, c.err
	}
	if c.bytesToReport > 0 {
		// read and discard at most bytesToReport to simulate partial drain
		buf := make([]byte, c.bytesToReport)
		n, _ := io.ReadFull(src, buf)
		return int64(n), nil
	}
	return io.Copy(dst, src)
}

/* -------------------- Tests -------------------- */

func TestAdapter_Write_Empty(t *testing.T) {
	ws := NewAdapterWSConnMock()
	a := NewAdapter(context.Background(), ws, nil)

	n, err := a.Write(nil)
	if n != 0 || err != nil {
		t.Fatalf("want (0,nil), got (%d,%v)", n, err)
	}
}

func TestAdapter_Write_Ok(t *testing.T) {
	ws := NewAdapterWSConnMock()
	// Default writer just buffers.
	a := NewAdapter(context.Background(), ws, nil)

	payload := []byte("hello world")
	n, err := a.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("write failed: n=%d err=%v", n, err)
	}
}

func TestAdapter_Write_PartialThenError(t *testing.T) {
	ws := NewAdapterWSConnMock()
	ws.writerFactory = func(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error) {
		return &AdapterWriteCloserMock{
			buf:       &bytes.Buffer{},
			failAfter: 3,
			writeErr:  errors.New("boom"),
		}, nil
	}
	a := NewAdapter(context.Background(), ws, nil)

	p := []byte{1, 2, 3, 4, 5}
	n, err := a.Write(p)
	if n != 3 || err == nil {
		t.Fatalf("expected partial=3 and error, got n=%d err=%v", n, err)
	}
}

func TestAdapter_Write_CloseError(t *testing.T) {
	ws := NewAdapterWSConnMock()
	ws.writerFactory = func(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error) {
		return &AdapterWriteCloserMock{
			buf:      &bytes.Buffer{},
			closeErr: errors.New("close-failed"),
		}, nil
	}
	a := NewAdapter(context.Background(), ws, nil)

	n, err := a.Write([]byte{1, 2, 3})
	if n != 3 || err == nil {
		t.Fatalf("expected n=3 and error from Close, got n=%d err=%v", n, err)
	}
}

func TestAdapter_Read_BinarySingleFrame(t *testing.T) {
	ws := NewAdapterWSConnMock()
	ws.EnqueueBinary([]byte{9, 8, 7})

	a := NewAdapter(context.Background(), ws, nil)
	buf := make([]byte, 10)
	n, err := a.Read(buf)
	if err != nil || n != 3 || !bytes.Equal(buf[:n], []byte{9, 8, 7}) {
		t.Fatalf("unexpected read: n=%d err=%v data=%v", n, err, buf[:n])
	}
}

func TestAdapter_Read_TextDrainedThenBinary(t *testing.T) {
	ws := NewAdapterWSConnMock()
	ws.EnqueueText([]byte("ignore me"))
	ws.EnqueueBinary([]byte{1, 2, 3})

	// Use mock copier to observe drain call (optional).
	copier := &AdapterCopierMock{}
	opts := &internal.Options{Copier: copier}
	a := NewAdapter(context.Background(), ws, opts)

	buf := make([]byte, 4)
	n, err := a.Read(buf)
	if err != nil || n != 3 || !bytes.Equal(buf[:n], []byte{1, 2, 3}) {
		t.Fatalf("unexpected read after text: n=%d err=%v data=%v", n, err, buf[:n])
	}
	if !copier.called {
		t.Fatalf("expected copier to be called for non-binary frame drain")
	}
}

func TestAdapter_Read_BinaryChunkedMultipleReads(t *testing.T) {
	ws := NewAdapterWSConnMock()
	ws.EnqueueBinary([]byte{1, 2, 3, 4, 5})

	a := NewAdapter(context.Background(), ws, nil)

	buf := make([]byte, 2)
	// first chunk
	n1, err := a.Read(buf)
	if err != nil || n1 != 2 || !bytes.Equal(buf[:n1], []byte{1, 2}) {
		t.Fatalf("chunk1 bad: n=%d err=%v data=%v", n1, err, buf[:n1])
	}
	// second chunk
	n2, err := a.Read(buf)
	if err != nil || n2 != 2 || !bytes.Equal(buf[:n2], []byte{3, 4}) {
		t.Fatalf("chunk2 bad: n=%d err=%v data=%v", n2, err, buf[:n2])
	}
	// last chunk
	n3, err := a.Read(buf)
	if err != nil || n3 != 1 || !bytes.Equal(buf[:n3], []byte{5}) {
		t.Fatalf("chunk3 bad: n=%d err=%v data=%v", n3, err, buf[:n3])
	}
}

func TestAdapter_Read_ErrorMapped_CloseNormalToEOF(t *testing.T) {
	ws := NewAdapterWSConnMock()
	ws.EnqueueErr(&websocket.CloseError{Code: websocket.StatusNormalClosure})

	a := NewAdapter(context.Background(), ws, nil)
	_, err := a.Read(make([]byte, 1))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestAdapter_Deadlines_ReadTimeout(t *testing.T) {
	ws := NewAdapterWSConnMock() // no frames; Reader will block until ctx done
	a := NewAdapter(context.Background(), ws, nil)

	_ = a.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	start := time.Now()
	_, err := a.Read(make([]byte, 1))
	if err == nil {
		t.Fatalf("expected deadline error")
	}
	if time.Since(start) < 8*time.Millisecond {
		t.Fatalf("deadline did not take effect")
	}
}

func TestAdapter_Deadlines_WriteTimeout(t *testing.T) {
	ws := NewAdapterWSConnMock()
	// Writer returned will block on Write until ctx.Done().
	ws.writerFactory = func(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error) {
		return &AdapterWriteCloserMock{
			buf:             &bytes.Buffer{},
			blockOnWriteCtx: ctx,
		}, nil
	}
	a := NewAdapter(context.Background(), ws, nil)

	_ = a.SetWriteDeadline(time.Now().Add(10 * time.Millisecond))
	start := time.Now()
	_, err := a.Write([]byte("payload"))
	if err == nil {
		t.Fatalf("expected deadline error from write")
	}
	if time.Since(start) < 8*time.Millisecond {
		t.Fatalf("write deadline did not take effect")
	}
}

func TestAdapter_WithAddrs_DefaultsAndOverride(t *testing.T) {
	ws := NewAdapterWSConnMock()
	a := NewAdapter(context.Background(), ws, nil)

	// defaults
	if _, ok := a.LocalAddr().(*net.TCPAddr); !ok {
		t.Fatalf("LocalAddr default should be *net.TCPAddr")
	}
	if _, ok := a.RemoteAddr().(*net.TCPAddr); !ok {
		t.Fatalf("RemoteAddr default should be *net.TCPAddr")
	}

	l := &net.TCPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 1111}
	r := &net.TCPAddr{IP: net.IPv4(5, 6, 7, 8), Port: 2222}
	a.WithAddrs(l, r)
	if a.LocalAddr().String() != l.String() || a.RemoteAddr().String() != r.String() {
		t.Fatalf("WithAddrs not applied")
	}
}

func TestAdapter_Close_Normal(t *testing.T) {
	ws := NewAdapterWSConnMock()
	a := NewAdapter(context.Background(), ws, nil)

	if err := a.Close(); err != nil {
		t.Fatalf("close returned error: %v", err)
	}
	if !ws.closeCalled || ws.closeCode != websocket.StatusNormalClosure {
		t.Fatalf("Close not propagated correctly")
	}
}
