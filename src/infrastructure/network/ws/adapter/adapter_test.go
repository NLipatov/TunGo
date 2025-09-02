package adapter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Compile-time check that Adapter implements net.Conn.
var _ net.Conn = &Adapter{}

// ---------- Test doubles (prefixed with Adapter...) ----------

// AdapterMockConn is a controllable mock for ws.Conn used by Adapter.
type AdapterMockConn struct {
	mu sync.Mutex

	// scripted factories
	writerFactory func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error)
	readerFactory func(ctx context.Context) (websocket.MessageType, io.Reader, error)

	// Close capture
	closeCode   websocket.StatusCode
	closeReason string
	closeCalls  int
	closeErr    error
}

func (m *AdapterMockConn) Writer(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
	m.mu.Lock()
	fn := m.writerFactory
	m.mu.Unlock()
	if fn == nil {
		return nil, errors.New("writerFactory not set")
	}
	return fn(ctx, mt)
}

func (m *AdapterMockConn) Reader(ctx context.Context) (websocket.MessageType, io.Reader, error) {
	m.mu.Lock()
	fn := m.readerFactory
	m.mu.Unlock()
	if fn == nil {
		return 0, nil, errors.New("readerFactory not set")
	}
	return fn(ctx)
}

func (m *AdapterMockConn) Close(code websocket.StatusCode, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCode = code
	m.closeReason = reason
	m.closeCalls++
	return m.closeErr
}

// AdapterMockWriteCloser simulates writes in chunks and optional errors.
type AdapterMockWriteCloser struct {
	mu sync.Mutex

	// chunks defines how many bytes to accept per Write call; if nil -> write all.
	chunks []int

	// optional write error: if set, error returned starting from write call index >= writeErrAt.
	writeErrAt *int
	writeErr   error

	// Close() error
	closeErr error

	// capture data
	writes      [][]byte
	ctxCaptured context.Context
}

func (w *AdapterMockWriteCloser) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	callIdx := len(w.writes)
	// determine amount to accept
	var n int
	if w.chunks == nil || callIdx >= len(w.chunks) {
		n = len(p)
	} else {
		n = w.chunks[callIdx]
		if n > len(p) {
			n = len(p)
		}
	}
	if n > 0 {
		cp := append([]byte(nil), p[:n]...)
		w.writes = append(w.writes, cp)
	} else {
		// record zero write to make assertions if needed
		w.writes = append(w.writes, nil)
	}

	// inject error?
	if w.writeErrAt != nil && callIdx >= *w.writeErrAt {
		return n, w.writeErr
	}
	return n, nil
}

func (w *AdapterMockWriteCloser) Close() error {
	return w.closeErr
}

// AdapterMockReader returns scripted chunks per Read and optional terminal error.
type AdapterMockReader struct {
	mu sync.Mutex

	chunks   [][]byte // each call returns next chunk; nil chunk means return 0, nil once
	errAtEnd error    // returned after chunks exhausted (if nil -> io.EOF)
}

func (r *AdapterMockReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.chunks) > 0 {
		ch := r.chunks[0]
		r.chunks = r.chunks[1:]
		if ch == nil {
			return 0, nil
		}
		n := copy(p, ch)
		return n, nil
	}
	if r.errAtEnd != nil {
		return 0, r.errAtEnd
	}
	return 0, io.EOF
}

// ---------- Helpers ----------

func mustDeadlineSet(t *testing.T, ctx context.Context) {
	t.Helper()
	if d, ok := ctx.Deadline(); !ok || d.IsZero() {
		t.Fatalf("expected context with deadline, got none")
	}
}
func mustNoDeadline(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, ok := ctx.Deadline(); ok {
		t.Fatalf("expected context without deadline, but got one")
	}
}

// ---------- Tests: Write ----------

func TestAdapter_Write_ZeroLen(t *testing.T) {
	a := NewDefaultAdapter(context.Background(), &AdapterMockConn{}, nil, nil)
	n, err := a.Write(nil)
	if n != 0 || err != nil {
		t.Fatalf("expected (0,nil), got (%d,%v)", n, err)
	}
}

func TestAdapter_Write_AllAtOnce_NoDeadline(t *testing.T) {
	wc := &AdapterMockWriteCloser{}
	conn := &AdapterMockConn{
		writerFactory: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			if mt != websocket.MessageBinary {
				t.Fatalf("expected MessageBinary")
			}
			mustNoDeadline(t, ctx)
			wc.ctxCaptured = ctx
			return wc, nil
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)

	data := []byte("hello world")
	n, err := a.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("write: got (%d,%v), want (%d,nil)", n, err, len(data))
	}
	if len(wc.writes) != 1 || !bytes.Equal(wc.writes[0], data) {
		t.Fatalf("unexpected writes: %#v", wc.writes)
	}
}

func TestAdapter_Write_PartialThenError(t *testing.T) {
	errBoom := errors.New("boom")
	idx := 1
	wc := &AdapterMockWriteCloser{
		chunks:     []int{3, 2}, // write 3 then 2
		writeErrAt: &idx,        // on 2nd Write call
		writeErr:   errBoom,
	}
	conn := &AdapterMockConn{
		writerFactory: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return wc, nil
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)

	data := []byte("abcdef")
	n, err := a.Write(data)
	if n != 5 {
		t.Fatalf("expected written=5, got %d", n)
	}
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestAdapter_Write_CloseError(t *testing.T) {
	wc := &AdapterMockWriteCloser{closeErr: errors.New("close failed")}
	conn := &AdapterMockConn{
		writerFactory: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return wc, nil
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)

	n, err := a.Write([]byte("abc"))
	if n != 3 || err == nil {
		t.Fatalf("want (3,err), got (%d,%v)", n, err)
	}
}

func TestAdapter_Write_RespectsWriteDeadline(t *testing.T) {
	wc := &AdapterMockWriteCloser{}
	captured := make(chan context.Context, 1)
	conn := &AdapterMockConn{
		writerFactory: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			captured <- ctx
			return wc, nil
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)
	_ = a.SetWriteDeadline(time.Now().Add(5 * time.Second))

	if _, err := a.Write([]byte("x")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	select {
	case ctx := <-captured:
		mustDeadlineSet(t, ctx)
	default:
		t.Fatalf("writerFactory wasn't invoked")
	}
}

func TestAdapter_Write_MapWriteErr_CloseErrorToNetErrClosed(t *testing.T) {
	// Writer returns CloseError on Write() to trigger mapWriteErr Close branch.
	closeErr := &websocket.CloseError{Code: websocket.StatusNormalClosure}
	idx := 0
	wc := &AdapterMockWriteCloser{
		chunks:     []int{0}, // write 0, but return error
		writeErrAt: &idx,
		writeErr:   closeErr,
	}
	conn := &AdapterMockConn{
		writerFactory: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return wc, nil
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)

	n, err := a.Write([]byte("abcd"))
	if n != 0 || !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected (0, net.ErrClosed), got (%d, %v)", n, err)
	}
}

func TestAdapter_Write_ErrNoProgressOnZeroNil(t *testing.T) {
	wc := &AdapterMockWriteCloser{
		chunks: []int{0}, // first write returns 0, nil
	}
	conn := &AdapterMockConn{
		writerFactory: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return wc, nil
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)
	n, err := a.Write([]byte("zzz"))
	if n != 0 || !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("expected (0, io.ErrNoProgress), got (%d,%v)", n, err)
	}
}

// ---------- Tests: Read ----------

func TestAdapter_Read_ZeroLen(t *testing.T) {
	a := NewDefaultAdapter(context.Background(), &AdapterMockConn{}, nil, nil)
	n, err := a.Read(nil)
	if n != 0 || err != nil {
		t.Fatalf("expected (0,nil), got (%d,%v)", n, err)
	}
}

func TestAdapter_Read_FromExistingReader_Success(t *testing.T) {
	r := &AdapterMockReader{chunks: [][]byte{[]byte("abc")}}
	a := NewAdapter(context.Background(), &AdapterMockConn{}, r, nil, nil, time.Time{}, time.Time{})
	buf := make([]byte, 8)
	n, err := a.Read(buf)
	if err != nil || string(buf[:n]) != "abc" {
		t.Fatalf("got (%d,%v,%q)", n, err, string(buf[:n]))
	}
}

func TestAdapter_Read_FromExistingReader_ZeroNil_NoProgress(t *testing.T) {
	r := &AdapterMockReader{chunks: [][]byte{nil}}
	a := NewAdapter(context.Background(), &AdapterMockConn{}, r, nil, nil, time.Time{}, time.Time{})
	_, err := a.Read(make([]byte, 8))
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("expected io.ErrNoProgress, got %v", err)
	}
}

func TestAdapter_Read_FromExistingReader_SomeThenEOF_SuppressesEOF(t *testing.T) {
	// Use bytes.Reader behavior: read returns (n>0, io.EOF) at the end.
	br := bytes.NewReader([]byte("hi"))
	a := NewAdapter(context.Background(), &AdapterMockConn{}, br, nil, nil, time.Time{}, time.Time{})
	buf := make([]byte, 8)
	n, err := a.Read(buf)
	if err != nil || string(buf[:n]) != "hi" {
		t.Fatalf("expected 'hi', err=nil; got (%d,%v,%q)", n, err, string(buf[:n]))
	}
}

func TestAdapter_Read_FromExistingReader_ErrorMapped(t *testing.T) {
	r := &AdapterMockReader{chunks: [][]byte{[]byte("a")}, errAtEnd: context.DeadlineExceeded}
	a := NewAdapter(context.Background(), &AdapterMockConn{}, r, nil, nil, time.Time{}, time.Time{})
	buf := make([]byte, 8)
	n, err := a.Read(buf)
	if n != 1 {
		t.Fatalf("expected n=1, got %d", n)
	}
	// After first call, second Read hits errAtEnd through adapter path:
	n, err = a.Read(buf)
	if n != 0 {
		t.Fatalf("expected n=0 on second read, got %d", n)
	}
	var ne interface{ Timeout() bool }
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("expected timeout net.Error, got %v", err)
	}
}

func TestAdapter_Read_ReaderFactoryError_MappedVariants(t *testing.T) {
	// Test mapping of CloseError -> io.EOF
	conn1 := &AdapterMockConn{
		readerFactory: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			return 0, nil, &websocket.CloseError{Code: websocket.StatusNormalClosure}
		},
	}
	a1 := NewDefaultAdapter(context.Background(), conn1, nil, nil)
	n, err := a1.Read(make([]byte, 8))
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("expected (0, io.EOF), got (%d, %v)", n, err)
	}

	// Abnormal closure -> io.ErrUnexpectedEOF
	conn2 := &AdapterMockConn{
		readerFactory: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			return 0, nil, &websocket.CloseError{Code: websocket.StatusAbnormalClosure}
		},
	}
	a2 := NewDefaultAdapter(context.Background(), conn2, nil, nil)
	n, err = a2.Read(make([]byte, 8))
	if n != 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected (0, io.ErrUnexpectedEOF), got (%d, %v)", n, err)
	}

	// DeadlineExceeded -> timeout net.Error
	conn3 := &AdapterMockConn{
		readerFactory: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			return 0, nil, context.DeadlineExceeded
		},
	}
	a3 := NewDefaultAdapter(context.Background(), conn3, nil, nil)
	_, err = a3.Read(make([]byte, 8))
	var ne interface{ Timeout() bool }
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("expected timeout net.Error, got %v", err)
	}
}

func TestAdapter_Read_NonBinaryFramesAreDrained_ThenBinaryRead(t *testing.T) {
	drained := false
	conn := &AdapterMockConn{
		readerFactory: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			// On first call return text frame, second call return binary frame.
			if !drained {
				// text frame with some payload to drain
				drained = true
				return websocket.MessageText, bytes.NewReader([]byte("ignore me")), nil
			}
			// binary frame
			return websocket.MessageBinary, bytes.NewReader([]byte("abc")), nil
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)
	buf := make([]byte, 8)
	n, err := a.Read(buf)
	if err != nil || string(buf[:n]) != "abc" {
		t.Fatalf("expected 'abc', got (%d,%v,%q)", n, err, string(buf[:n]))
	}
}

func TestAdapter_Read_RespectsReadDeadlineOnFrameCtx(t *testing.T) {
	captured := make(chan context.Context, 1)
	conn := &AdapterMockConn{
		readerFactory: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			captured <- ctx
			// return an error so adapter returns quickly
			return 0, nil, context.DeadlineExceeded
		},
	}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)
	_ = a.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _ = a.Read(make([]byte, 1))
	select {
	case ctx := <-captured:
		mustDeadlineSet(t, ctx)
	default:
		t.Fatalf("readerFactory wasn't invoked")
	}
}

// ---------- Tests: cancelOnEOF ----------

func TestAdapter_cancelOnEOF_CallsCancelExactlyOnce(t *testing.T) {
	var calls int
	c := &cancelOnEOF{
		r: io.MultiReader(bytes.NewReader([]byte("x")), bytes.NewReader(nil)), // will hit EOF after read
		cancel: func() {
			calls++
		},
	}
	buf := make([]byte, 8)
	// first read: returns "x", nil
	n, err := c.Read(buf)
	if err != nil || n != 1 {
		t.Fatalf("expected (1,nil), got (%d,%v)", n, err)
	}
	// second read: returns 0, EOF -> triggers cancel
	n, err = c.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("expected (0,EOF), got (%d,%v)", n, err)
	}
	// third read: ensure cancel not called again even if EOF repeats
	_, _ = c.Read(buf)
	if calls != 1 {
		t.Fatalf("expected cancel() to be called once, got %d", calls)
	}
}

// ---------- Tests: Close & addresses ----------

func TestAdapter_Close_NormalClosure(t *testing.T) {
	conn := &AdapterMockConn{}
	a := NewDefaultAdapter(context.Background(), conn, nil, nil)
	if err := a.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.closeCalls != 1 || conn.closeCode != websocket.StatusNormalClosure || conn.closeReason != "" {
		t.Fatalf("unexpected close capture: calls=%d code=%d reason=%q", conn.closeCalls, conn.closeCode, conn.closeReason)
	}
}

func TestAdapter_LocalAddr_DefaultAndCustom(t *testing.T) {
	a := NewDefaultAdapter(context.Background(), &AdapterMockConn{}, nil, nil)
	if _, ok := a.LocalAddr().(*net.TCPAddr); !ok {
		t.Fatalf("expected default *net.TCPAddr")
	}
	custom := &net.TCPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 42}
	a.lAddr = custom
	if a.LocalAddr() != custom {
		t.Fatalf("expected custom local addr")
	}
}

func TestAdapter_RemoteAddr_DefaultAndCustom(t *testing.T) {
	a := NewDefaultAdapter(context.Background(), &AdapterMockConn{}, nil, nil)
	if _, ok := a.RemoteAddr().(*net.TCPAddr); !ok {
		t.Fatalf("expected default *net.TCPAddr")
	}
	custom := &net.TCPAddr{IP: net.IPv4(5, 6, 7, 8), Port: 43}
	a.rAddr = custom
	if a.RemoteAddr() != custom {
		t.Fatalf("expected custom remote addr")
	}
}

// ---------- Tests: Deadlines setters ----------

func TestAdapter_SetDeadlines(t *testing.T) {
	a := NewDefaultAdapter(context.Background(), &AdapterMockConn{}, nil, nil)

	d := time.Now().Add(10 * time.Second)
	if err := a.SetDeadline(d); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if a.readDeadline != d || a.writeDeadline != d {
		t.Fatalf("deadline not set correctly")
	}

	r := time.Now().Add(3 * time.Second)
	if err := a.SetReadDeadline(r); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if a.readDeadline != r {
		t.Fatalf("read deadline not set")
	}

	w := time.Now().Add(4 * time.Second)
	if err := a.SetWriteDeadline(w); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}
	if a.writeDeadline != w {
		t.Fatalf("write deadline not set")
	}
}

// ---------- Tests: map*Err & errTimeout ----------

func TestAdapter_mapReadErr_Variants(t *testing.T) {
	a := NewDefaultAdapter(context.Background(), &AdapterMockConn{}, nil, nil)

	if got := a.mapReadErr(nil); got != nil {
		t.Fatalf("nil -> %v", got)
	}
	if !errors.Is(a.mapReadErr(&websocket.CloseError{Code: websocket.StatusNormalClosure}), io.EOF) {
		t.Fatalf("NormalClosure must map to EOF")
	}
	if !errors.Is(a.mapReadErr(&websocket.CloseError{Code: websocket.StatusGoingAway}), io.EOF) {
		t.Fatalf("GoingAway must map to EOF")
	}
	if !errors.Is(a.mapReadErr(&websocket.CloseError{Code: websocket.StatusAbnormalClosure}), io.ErrUnexpectedEOF) {
		t.Fatalf("AbnormalClosure must map to UnexpectedEOF")
	}
	if !errors.Is(a.mapReadErr(&websocket.CloseError{Code: websocket.StatusNoStatusRcvd}), io.ErrUnexpectedEOF) {
		t.Fatalf("NoStatusRcvd must map to UnexpectedEOF")
	}
	err := a.mapReadErr(context.DeadlineExceeded)
	var ne interface{ Timeout() bool }
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("DeadlineExceeded must map to net.Error Timeout")
	}
	other := errors.New("x")
	if a.mapReadErr(other) != other {
		t.Fatalf("other errors must remain unchanged")
	}
}

func TestAdapter_mapWriteErr_Variants(t *testing.T) {
	a := NewDefaultAdapter(context.Background(), &AdapterMockConn{}, nil, nil)

	if got := a.mapWriteErr(nil); got != nil {
		t.Fatalf("nil -> %v", got)
	}
	if !errors.Is(a.mapWriteErr(&websocket.CloseError{Code: websocket.StatusNormalClosure}), net.ErrClosed) {
		t.Fatalf("CloseError must map to net.ErrClosed")
	}
	err := a.mapWriteErr(context.DeadlineExceeded)
	var ne interface{ Timeout() bool }
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("DeadlineExceeded must map to net.Error Timeout")
	}
	other := errors.New("y")
	if a.mapWriteErr(other) != other {
		t.Fatalf("other errors must remain unchanged")
	}
}

func TestErrTimeout_ImplementsNetError(t *testing.T) {
	cause := context.DeadlineExceeded
	e := errTimeout{cause: cause}
	if e.Error() == "" {
		t.Fatalf("empty Error string")
	}
	if !errors.Is(e, cause) {
		t.Fatalf("Unwrap must expose cause")
	}
	type timeoutIface interface {
		Timeout() bool
	}
	var ne timeoutIface
	if !errors.As(e, &ne) || !ne.Timeout() {
		t.Fatalf("errTimeout must satisfy net.Error Timeout()==true")
	}
}
