package adapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	networkpkg "tungo/domain/network"

	"github.com/coder/websocket"
)

/************* Mocks (prefix: Adapter...) *************/

// AdapterWSConnMock mocks ws.Conn used by Adapter.
type AdapterWSConnMock struct {
	writerFn func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error)
	readerFn func(ctx context.Context) (websocket.MessageType, io.Reader, error)
	closeFn  func(code websocket.StatusCode, reason string) error
}

func (m *AdapterWSConnMock) Writer(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
	return m.writerFn(ctx, mt)
}
func (m *AdapterWSConnMock) Reader(ctx context.Context) (websocket.MessageType, io.Reader, error) {
	return m.readerFn(ctx)
}
func (m *AdapterWSConnMock) Close(code websocket.StatusCode, reason string) error {
	if m.closeFn != nil {
		return m.closeFn(code, reason)
	}
	return nil
}

// AdapterWriteCloserMock simulates partial writes and errors.
type AdapterWriteCloserMock struct {
	buf          *bytes.Buffer
	chunk        int   // first Write will cap to chunk bytes if >0
	failOn2nd    error // error on the SECOND Write call
	failOnWrite  error // immediate error on any Write if set (used in writer-error test)
	failOnClose  error
	writeCalls   int
	closeCalls   int
	sleepOnWrite time.Duration
}

func (w *AdapterWriteCloserMock) Write(p []byte) (int, error) {
	// Immediate write error mode (used by Writer() error path tests)
	if w.failOnWrite != nil {
		return 0, w.failOnWrite
	}
	w.writeCalls++
	if w.sleepOnWrite > 0 {
		time.Sleep(w.sleepOnWrite)
	}
	// Simulate partial write on first call, then error on second call.
	if w.chunk > 0 && w.writeCalls == 1 {
		n, _ := w.buf.Write(p[:min(w.chunk, len(p))])
		return n, nil
	}
	if w.failOn2nd != nil && w.writeCalls == 2 {
		return 0, w.failOn2nd
	}
	return w.buf.Write(p)
}
func (w *AdapterWriteCloserMock) Close() error {
	w.closeCalls++
	return w.failOnClose
}

// AdapterErrorMapperMock verifies that errors pass through the mapper.
type AdapterErrorMapperMock struct{ prefix string }

func (m AdapterErrorMapperMock) mapErr(err error) error {
	return fmt.Errorf("%s:%v", m.prefix, err)
}

// errReader returns (n, err) once, then EOF forever.
type errReader struct {
	data     []byte
	err      error
	consumed bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.consumed {
		return 0, io.EOF
	}
	r.consumed = true
	n := copy(p, r.data)
	return n, r.err
}

// alwaysErrReader always returns the same error.
type alwaysErrReader struct{ err error }

func (r alwaysErrReader) Read(_ []byte) (int, error) { return 0, r.err }

// dummy net.Addr
type dummyAddr struct{ s string }

func (d dummyAddr) Network() string { return "dummy" }
func (d dummyAddr) String() string  { return d.s }

/******************** Tests ********************/

func TestNewDefaultAdapter_ZeroDeadlines(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{
		writerFn: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return &AdapterWriteCloserMock{buf: &bytes.Buffer{}}, nil
		},
		readerFn: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			return websocket.MessageBinary, bytes.NewBuffer(nil), nil
		},
	}, nil, nil)
	if !ad.readDeadline.ExpiresAt().IsZero() || !ad.writeDeadline.ExpiresAt().IsZero() {
		t.Fatal("expected zero (disabled) deadlines by default")
	}
}

func TestNewAdapter_FieldsAssigned(t *testing.T) {
	t.Parallel()
	em := AdapterErrorMapperMock{prefix: "x"}
	l := &dummyAddr{"l"}
	r := &dummyAddr{"r"}
	ad := NewAdapter(&AdapterWSConnMock{}, context.Background(), em, nil, l, r,
		networkDeadlineZero(t), networkDeadlineZero(t))
	if ad.lAddr != l || ad.rAddr != r {
		t.Fatal("addresses not assigned")
	}
}

func TestWrite_ZeroLen(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)
	n, err := ad.Write(nil)
	if n != 0 || err != nil {
		t.Fatalf("got (%d,%v), want (0,nil)", n, err)
	}
}

func TestWrite_NoDeadline_Success(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	w := &AdapterWriteCloserMock{buf: buf}
	mock := &AdapterWSConnMock{
		writerFn: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return w, nil
		},
	}
	ad := NewDefaultAdapter(context.Background(), mock, nil, nil)
	data := []byte("hello")
	n, err := ad.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("got (%d,%v), want (%d,nil)", n, err, len(data))
	}
	if buf.String() != "hello" {
		t.Fatalf("buffer=%q", buf.String())
	}
	if w.closeCalls != 1 {
		t.Fatalf("writer.Close calls = %d, want 1", w.closeCalls)
	}
}

func TestWrite_WithDeadline_WriterErrorMapped(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("boom")
	mock := &AdapterWSConnMock{
		writerFn: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return nil, wantErr
		},
	}
	em := AdapterErrorMapperMock{prefix: "mapped"}
	ad := NewAdapter(mock, context.Background(), em, nil, nil, nil,
		networkDeadlineZero(t), networkDeadlineFuture(t))
	_, err := ad.Write([]byte("x"))
	if err == nil || err.Error() != "mapped:boom" {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestWrite_PartialThenError(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	w := &AdapterWriteCloserMock{
		buf:       buf,
		chunk:     2,                        // first call writes 2 bytes
		failOn2nd: errors.New("fail-write"), // second call errors
	}
	mock := &AdapterWSConnMock{
		writerFn: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return w, nil
		},
	}
	em := AdapterErrorMapperMock{prefix: "map"}
	ad := NewAdapter(mock, context.Background(), em, nil, nil, nil,
		networkDeadlineZero(t), networkDeadlineZero(t))

	n, err := ad.Write([]byte("abcd"))
	if n != 2 || err == nil || err.Error() != "map:fail-write" {
		t.Fatalf("got (n=%d, err=%v), want (2, map:fail-write)", n, err)
	}
	// On error path, deferred Close() is still called once.
	if w.closeCalls != 1 {
		t.Fatalf("writer.Close calls = %d, want 1 (deferred on error path)", w.closeCalls)
	}
}

func TestWrite_CloseErrorAfterSuccess(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	w := &AdapterWriteCloserMock{buf: buf, failOnClose: errors.New("close-fail")}
	mock := &AdapterWSConnMock{
		writerFn: func(ctx context.Context, mt websocket.MessageType) (io.WriteCloser, error) {
			return w, nil
		},
	}
	em := AdapterErrorMapperMock{prefix: "emap"}
	ad := NewAdapter(mock, context.Background(), em, nil, nil, nil,
		networkDeadlineZero(t), networkDeadlineZero(t))
	n, err := ad.Write([]byte("xyz"))
	if n != 3 || err == nil || err.Error() != "emap:close-fail" {
		t.Fatalf("got (n=%d, err=%v), want (3, emap:close-fail)", n, err)
	}
	// Because explicit Close() failed, deferred Close() runs too => 2 calls.
	if w.closeCalls != 2 {
		t.Fatalf("writer.Close calls = %d, want 2 (explicit + deferred)", w.closeCalls)
	}
}

func TestRead_ZeroLen(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)
	n, err := ad.Read(nil)
	if n != 0 || err != nil {
		t.Fatalf("got (%d,%v), want (0,nil)", n, err)
	}
}

func TestRead_ExistingReader_Success(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)
	ad.reader = bytes.NewBufferString("abc")
	buf := make([]byte, 2)
	n, err := ad.Read(buf)
	if err != nil || n != 2 || string(buf[:n]) != "ab" {
		t.Fatalf("got (n=%d, err=%v, data=%q)", n, err, string(buf[:n]))
	}
}

func TestRead_ExistingReader_EOFWithBytes(t *testing.T) {
	t.Parallel()
	cancelCount := 0
	// Return n>0 with io.EOF in the same Read call so cancelOnEOF triggers.
	wrapped := &cancelOnEOF{
		r:      &errReader{data: []byte("ok"), err: io.EOF},
		cancel: func() { cancelCount++ },
	}
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)
	ad.reader = wrapped

	buf := make([]byte, 8)
	n, err := ad.Read(buf)
	if err != nil || n != 2 || string(buf[:n]) != "ok" {
		t.Fatalf("got (n=%d, err=%v, data=%q)", n, err, string(buf[:n]))
	}
	if cancelCount != 1 {
		t.Fatalf("cancel called %d times, want 1", cancelCount)
	}
	if ad.reader != nil {
		t.Fatal("expected reader to be cleared on EOF")
	}
}

func TestRead_ExistingReader_ErrorMapped(t *testing.T) {
	t.Parallel()
	em := AdapterErrorMapperMock{prefix: "mapped"}
	inner := &errReader{data: []byte("qq"), err: errors.New("r-fail")}
	ad := NewAdapter(&AdapterWSConnMock{}, context.Background(), em, &cancelOnEOF{
		r:      inner,
		cancel: func() {},
	}, nil, nil, networkDeadlineZero(t), networkDeadlineZero(t))

	buf := make([]byte, 8)
	n, err := ad.Read(buf)
	if n != 2 || err == nil || err.Error() != "mapped:r-fail" {
		t.Fatalf("got (n=%d, err=%v), want (2, mapped:r-fail)", n, err)
	}
	if ad.reader != nil {
		t.Fatal("expected reader cleared on error")
	}
}

func TestRead_ConnReaderError_Mapped(t *testing.T) {
	t.Parallel()
	em := AdapterErrorMapperMock{prefix: "me"}
	mock := &AdapterWSConnMock{
		readerFn: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			return 0, nil, errors.New("conn-read")
		},
	}
	ad := NewAdapter(mock, context.Background(), em, nil, nil, nil,
		networkDeadlineZero(t), networkDeadlineZero(t))
	buf := make([]byte, 1)
	_, err := ad.Read(buf)
	if err == nil || err.Error() != "me:conn-read" {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestRead_DrainNonBinaryThenBinary(t *testing.T) {
	t.Parallel()
	seq := 0
	mock := &AdapterWSConnMock{
		readerFn: func(ctx context.Context) (websocket.MessageType, io.Reader, error) {
			if seq == 0 {
				seq++
				return websocket.MessageText, bytes.NewBufferString("ignore"), nil
			}
			return websocket.MessageBinary, bytes.NewBuffer([]byte("OK")), nil
		},
	}
	ad := NewDefaultAdapter(context.Background(), mock, nil, nil)
	buf := make([]byte, 4)
	n, err := ad.Read(buf)
	if err != nil || string(buf[:n]) != "OK" {
		t.Fatalf("got (n=%d, err=%v, data=%q)", n, err, string(buf[:n]))
	}
}

func TestClose_DelegatesToConn(t *testing.T) {
	t.Parallel()
	var gotCode websocket.StatusCode
	var gotReason string
	mock := &AdapterWSConnMock{
		closeFn: func(code websocket.StatusCode, reason string) error {
			gotCode, gotReason = code, reason
			return nil
		},
	}
	ad := NewDefaultAdapter(context.Background(), mock, nil, nil)
	if err := ad.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}
	if gotCode != websocket.StatusNormalClosure || gotReason != "" {
		t.Fatalf("close args = (%v,%q), want (StatusNormalClosure, \"\")", gotCode, gotReason)
	}
}

func TestLocalAddr_DefaultAndSet(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)
	if _, ok := ad.LocalAddr().(*net.TCPAddr); !ok {
		t.Fatalf("expected default TCPAddr when nil")
	}
	l := &dummyAddr{"L"}
	ad = NewAdapter(&AdapterWSConnMock{}, context.Background(), AdapterErrorMapperMock{}, nil, l, nil,
		networkDeadlineZero(t), networkDeadlineZero(t))
	if ad.LocalAddr() != l {
		t.Fatal("expected provided local addr")
	}
}

func TestRemoteAddr_DefaultAndSet(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)
	if _, ok := ad.RemoteAddr().(*net.TCPAddr); !ok {
		t.Fatalf("expected default TCPAddr when nil")
	}
	r := &dummyAddr{"R"}
	ad = NewAdapter(&AdapterWSConnMock{}, context.Background(), AdapterErrorMapperMock{}, nil, nil, r,
		networkDeadlineZero(t), networkDeadlineZero(t))
	if ad.RemoteAddr() != r {
		t.Fatal("expected provided remote addr")
	}
}

func TestSetDeadline_Various(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)

	// zero (clear)
	if err := ad.SetDeadline(time.Time{}); err != nil {
		t.Fatalf("zero deadline should be accepted, got %v", err)
	}
	if !ad.readDeadline.ExpiresAt().IsZero() || !ad.writeDeadline.ExpiresAt().IsZero() {
		t.Fatal("deadlines should be cleared to zero")
	}

	// past -> error
	if err := ad.SetDeadline(time.Now().Add(-time.Second)); err == nil {
		t.Fatal("expected error for past deadline")
	}

	// future -> ok
	if err := ad.SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetReadDeadline_Various(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)

	if err := ad.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("zero should clear: %v", err)
	}
	if err := ad.SetReadDeadline(time.Now().Add(-time.Second)); err == nil {
		t.Fatal("expected error for past")
	}
	if err := ad.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestSetWriteDeadline_Various(t *testing.T) {
	t.Parallel()
	ad := NewDefaultAdapter(context.Background(), &AdapterWSConnMock{}, nil, nil)

	if err := ad.SetWriteDeadline(time.Time{}); err != nil {
		t.Fatalf("zero should clear: %v", err)
	}
	if err := ad.SetWriteDeadline(time.Now().Add(-time.Second)); err == nil {
		t.Fatal("expected error for past")
	}
	if err := ad.SetWriteDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCancelOnEOF_Idempotent(t *testing.T) {
	t.Parallel()
	calls := 0
	c := &cancelOnEOF{
		r:      alwaysErrReader{err: io.EOF},
		cancel: func() { calls++ },
	}
	buf := make([]byte, 1)
	_, _ = c.Read(buf) // triggers EOF -> cancel
	_, _ = c.Read(buf) // triggers EOF again -> cancel should not increment
	if calls != 1 {
		t.Fatalf("cancel invoked %d times, want 1", calls)
	}
}

/************* helpers *************/

func networkDeadlineZero(t *testing.T) networkpkg.Deadline {
	t.Helper()
	dl, err := networkpkg.DeadlineFromTime(time.Time{})
	if err != nil {
		t.Fatalf("zero deadline factory err: %v", err)
	}
	return dl
}

func networkDeadlineFuture(t *testing.T) networkpkg.Deadline {
	t.Helper()
	dl, err := networkpkg.DeadlineFromTime(time.Now().Add(50 * time.Millisecond))
	if err != nil {
		t.Fatalf("future deadline factory err: %v", err)
	}
	return dl
}
