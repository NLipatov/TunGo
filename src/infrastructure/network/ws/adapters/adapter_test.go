package adapters

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/coder/websocket"
)

var _ WSConn = (*AdapterWSConnMock)(nil)

type AdapterWSConnMock struct {
	Frames []struct {
		Type websocket.MessageType
		Data []byte
		Err  error
	}
	readerCalls int

	Written [][]byte

	OnReader func(ctx context.Context, typ websocket.MessageType)
	OnWriter func(ctx context.Context, typ websocket.MessageType)

	WriterWriteErr error
	WriterCloseErr error
	PartialWriteN  int

	CloseCalls []struct {
		Status websocket.StatusCode
		Reason string
	}
}

func (m *AdapterWSConnMock) Reader(ctx context.Context) (websocket.MessageType, io.Reader, error) {
	if m.readerCalls >= len(m.Frames) {
		return websocket.MessageBinary, nil, io.EOF
	}
	f := m.Frames[m.readerCalls]
	m.readerCalls++

	if m.OnReader != nil {
		m.OnReader(ctx, f.Type)
	}

	if f.Err != nil {
		return 0, nil, f.Err
	}
	return f.Type, bytes.NewReader(f.Data), nil
}

func (m *AdapterWSConnMock) Writer(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error) {
	if m.OnWriter != nil {
		m.OnWriter(ctx, typ)
	}
	var buf bytes.Buffer
	m.Written = append(m.Written, nil)
	idx := len(m.Written) - 1

	// snapshot behavior at creation time
	writeErr := m.WriterWriteErr
	closeErr := m.WriterCloseErr
	partial := m.PartialWriteN

	return &bufferWriteCloser{
		writeFn: func(p []byte) (int, error) {
			if partial > 0 {
				n := partial
				if n > len(p) {
					n = len(p)
				}
				_, _ = buf.Write(p[:n])
				partial = 0
				if writeErr != nil {
					return n, writeErr
				}
				return n, nil
			}
			if writeErr != nil {
				return 0, writeErr
			}
			return buf.Write(p)
		},
		closeFn: func() error {
			if closeErr != nil {
				return closeErr
			}
			m.Written[idx] = append([]byte(nil), buf.Bytes()...)
			return nil
		},
	}, nil
}

func (m *AdapterWSConnMock) Close(status websocket.StatusCode, reason string) error {
	m.CloseCalls = append(m.CloseCalls, struct {
		Status websocket.StatusCode
		Reason string
	}{status, reason})
	return nil
}

type bufferWriteCloser struct {
	writeFn func([]byte) (int, error)
	closeFn func() error
}

func (b *bufferWriteCloser) Write(p []byte) (int, error) { return b.writeFn(p) }
func (b *bufferWriteCloser) Close() error                { return b.closeFn() }

func approxEqualTime(t1, t2 time.Time, d time.Duration) bool {
	if t1.IsZero() || t2.IsZero() {
		return false
	}
	diff := t1.Sub(t2)
	if diff < 0 {
		diff = -diff
	}
	return diff <= d
}

func TestAdapter_Write_ZeroLength(t *testing.T) {
	mock := &AdapterWSConnMock{}
	a := NewAdapter(context.Background(), mock)

	n, err := a.Write(nil)
	if err != nil || n != 0 {
		t.Fatalf("got n=%d err=%v, want 0, nil", n, err)
	}
	if len(mock.Written) != 0 {
		t.Fatalf("writer should not be created for zero-length write")
	}
}

func TestAdapter_Write_Success(t *testing.T) {
	mock := &AdapterWSConnMock{}
	a := NewAdapter(context.Background(), mock)

	data := []byte("hello world")
	n, err := a.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("write: got n=%d err=%v, want %d, nil", n, err, len(data))
	}
	if got := mock.Written; len(got) != 1 || !bytes.Equal(got[0], data) {
		t.Fatalf("written mismatch: %#v", got)
	}
}

func TestAdapter_Write_PartialThenError(t *testing.T) {
	mock := &AdapterWSConnMock{
		PartialWriteN:  3,
		WriterWriteErr: errors.New("boom"),
	}
	a := NewAdapter(context.Background(), mock)

	data := []byte("abcdef")
	n, err := a.Write(data)
	if n != 3 {
		t.Fatalf("want partial n=3, got %d", n)
	}
	if err == nil || err.Error() != "boom" {
		t.Fatalf("want error boom, got %v", err)
	}
}

func TestAdapter_Write_CloseError(t *testing.T) {
	mock := &AdapterWSConnMock{
		WriterCloseErr: errors.New("close-fail"),
	}
	a := NewAdapter(context.Background(), mock)

	data := []byte("x")
	n, err := a.Write(data)
	if n != 1 {
		t.Fatalf("want n=1, got %d", n)
	}
	if err == nil || err.Error() != "close-fail" {
		t.Fatalf("want close-fail, got %v", err)
	}
}

func TestAdapter_Read_SkipNonBinary(t *testing.T) {
	mock := &AdapterWSConnMock{
		Frames: []struct {
			Type websocket.MessageType
			Data []byte
			Err  error
		}{
			{Type: websocket.MessageText, Data: []byte("ignore"), Err: nil},
			{Type: websocket.MessageBinary, Data: []byte("ok"), Err: nil},
		},
	}
	a := NewAdapter(context.Background(), mock)

	buf := make([]byte, 10)
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}
	if string(buf[:n]) != "ok" {
		t.Fatalf("got %q, want %q", string(buf[:n]), "ok")
	}
}

func TestAdapter_Read_BinaryChunkedEOF(t *testing.T) {
	payload := []byte("hello world")
	mock := &AdapterWSConnMock{
		Frames: []struct {
			Type websocket.MessageType
			Data []byte
			Err  error
		}{
			{Type: websocket.MessageBinary, Data: payload},
		},
	}
	a := NewAdapter(context.Background(), mock)

	buf := make([]byte, 5)
	var out []byte

	n1, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read1 err: %v", err)
	}
	out = append(out, buf[:n1]...)

	n2, err := a.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read2 err: %v", err)
	}
	out = append(out, buf[:n2]...)

	if !bytes.Equal(out, payload[:len(out)]) {
		t.Fatalf("assembled %q, want prefix of %q", string(out), string(payload))
	}
	// Drain remaining bytes if any
	rest := make([]byte, 32)
	n3, _ := a.Read(rest)
	out = append(out, rest[:n3]...)
	if !bytes.Equal(out, payload) {
		t.Fatalf("final %q, want %q", string(out), string(payload))
	}
}

func TestAdapter_Read_ReaderCloseNormalMapsEOF(t *testing.T) {
	mock := &AdapterWSConnMock{
		Frames: []struct {
			Type websocket.MessageType
			Data []byte
			Err  error
		}{
			{Err: &websocket.CloseError{Code: websocket.StatusNormalClosure}},
		},
	}
	a := NewAdapter(context.Background(), mock)

	_, err := a.Read(make([]byte, 1))
	if err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

func TestAdapter_Read_ErrorPassthrough(t *testing.T) {
	exp := errors.New("reader-fail")
	mock := &AdapterWSConnMock{
		Frames: []struct {
			Type websocket.MessageType
			Data []byte
			Err  error
		}{
			{Err: exp},
		},
	}
	a := NewAdapter(context.Background(), mock)

	_, err := a.Read(make([]byte, 1))
	if !errors.Is(err, exp) {
		t.Fatalf("want %v, got %v", exp, err)
	}
}

func TestAdapter_Deadlines_Propagate(t *testing.T) {
	mock := &AdapterWSConnMock{}
	a := NewAdapter(context.Background(), mock)

	readDeadline := time.Now().Add(200 * time.Millisecond)
	writeDeadline := time.Now().Add(300 * time.Millisecond)

	mock.OnReader = func(ctx context.Context, _ websocket.MessageType) {
		dl, ok := ctx.Deadline()
		if !ok || !approxEqualTime(dl, readDeadline, 10*time.Millisecond) {
			t.Errorf("read deadline mismatch: got %v ok=%v, want ~%v", dl, ok, readDeadline)
		}
	}
	mock.OnWriter = func(ctx context.Context, _ websocket.MessageType) {
		dl, ok := ctx.Deadline()
		if !ok || !approxEqualTime(dl, writeDeadline, 10*time.Millisecond) {
			t.Errorf("write deadline mismatch: got %v ok=%v, want ~%v", dl, ok, writeDeadline)
		}
	}

	if err := a.SetReadDeadline(readDeadline); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if err := a.SetWriteDeadline(writeDeadline); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}

	mock.Frames = []struct {
		Type websocket.MessageType
		Data []byte
		Err  error
	}{
		{Type: websocket.MessageBinary, Data: []byte("x")},
	}

	_, _ = a.Read(make([]byte, 1))
	_, _ = a.Write([]byte("y"))
}

func TestAdapter_SetDeadline_Both(t *testing.T) {
	mock := &AdapterWSConnMock{}
	a := NewAdapter(context.Background(), mock)

	deadline := time.Now().Add(150 * time.Millisecond)

	var sawRead, sawWrite time.Time

	mock.OnReader = func(ctx context.Context, _ websocket.MessageType) {
		dl, _ := ctx.Deadline()
		sawRead = dl
	}
	mock.OnWriter = func(ctx context.Context, _ websocket.MessageType) {
		dl, _ := ctx.Deadline()
		sawWrite = dl
	}

	if err := a.SetDeadline(deadline); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	mock.Frames = []struct {
		Type websocket.MessageType
		Data []byte
		Err  error
	}{
		{Type: websocket.MessageBinary, Data: []byte("x")},
	}

	_, _ = a.Read(make([]byte, 1))
	_, _ = a.Write([]byte("y"))

	if !approxEqualTime(sawRead, deadline, 10*time.Millisecond) || !approxEqualTime(sawWrite, deadline, 10*time.Millisecond) {
		t.Fatalf("deadlines mismatch: read=%v write=%v want ~%v", sawRead, sawWrite, deadline)
	}
}

func TestAdapter_Addrs_DefaultAndCustom(t *testing.T) {
	mock := &AdapterWSConnMock{}
	a := NewAdapter(context.Background(), mock)

	if _, ok := a.LocalAddr().(*net.TCPAddr); !ok {
		t.Fatalf("default LocalAddr is not *net.TCPAddr")
	}
	if _, ok := a.RemoteAddr().(*net.TCPAddr); !ok {
		t.Fatalf("default RemoteAddr is not *net.TCPAddr")
	}

	l := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1111}
	r := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	a.WithAddrs(l, r)

	if !reflect.DeepEqual(a.LocalAddr(), l) || !reflect.DeepEqual(a.RemoteAddr(), r) {
		t.Fatalf("custom addrs mismatch")
	}
}

func TestAdapter_Close_Normal(t *testing.T) {
	mock := &AdapterWSConnMock{}
	a := NewAdapter(context.Background(), mock)

	if err := a.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}
	if len(mock.CloseCalls) != 1 || mock.CloseCalls[0].Status != websocket.StatusNormalClosure {
		t.Fatalf("unexpected close calls: %#v", mock.CloseCalls)
	}
}
