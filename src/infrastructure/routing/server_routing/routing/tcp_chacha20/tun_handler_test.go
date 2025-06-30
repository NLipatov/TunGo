package tcp_chacha20

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"testing"
	"time"
	"tungo/infrastructure/cryptography/chacha20"
)

type mockReader struct {
	seq [][]byte
	err []error
	i   int
}

func (r *mockReader) Read(p []byte) (int, error) {
	if r.i >= len(r.seq) {
		return 0, io.EOF
	}
	n := copy(p, r.seq[r.i])
	e := r.err[r.i]
	r.i++
	return n, e
}

type mockEncoder struct {
	called int32
	err    error
}

func (e *mockEncoder) Decode([]byte, *chacha20.TCPPacket) error { return nil }
func (e *mockEncoder) Encode([]byte) error                      { atomic.AddInt32(&e.called, 1); return e.err }

type mockParser struct{ err error }

func (p *mockParser) ParseDestinationAddressBytes(_, _ []byte) error { return p.err }

type mockCrypto struct{ err error }

func (m *mockCrypto) Encrypt([]byte) ([]byte, error) { return nil, m.err }
func (m *mockCrypto) Decrypt([]byte) ([]byte, error) { return nil, nil }

type mockConn struct {
	called int32
	err    error
}

func (c *mockConn) Write(b []byte) (int, error)        { atomic.AddInt32(&c.called, 1); return len(b), c.err }
func (c *mockConn) Read([]byte) (int, error)           { return 0, nil }
func (c *mockConn) Close() error                       { return nil }
func (c *mockConn) LocalAddr() net.Addr                { return nil }
func (c *mockConn) RemoteAddr() net.Addr               { return nil }
func (c *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

type mockMgr struct {
	sess    Session
	getErr  error
	deleted int32
}

func (m *mockMgr) Add(Session)                                   {}
func (m *mockMgr) Delete(Session)                                { atomic.AddInt32(&m.deleted, 1) }
func (m *mockMgr) GetByInternalIP(_ netip.Addr) (Session, error) { return m.sess, m.getErr }
func (m *mockMgr) GetByExternalIP(_ netip.Addr) (Session, error) { return m.sess, nil }

func makeSession(c *mockConn, crypto *mockCrypto) Session {
	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddr("2.2.2.2")
	return Session{
		conn:                c,
		CryptographyService: crypto,
		internalIP:          in,
		externalIP:          ex,
	}
}

func TestTunHandler_AllPaths(t *testing.T) {
	buf := make([]byte, 16)
	reader := func(seq [][]byte, err []error) io.Reader { return &mockReader{seq: seq, err: err} }

	t.Run("context done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		h := NewTunHandler(ctx, reader(nil, nil), &mockEncoder{}, &mockParser{}, &mockMgr{})
		if err := h.HandleTun(); err != nil {
			t.Errorf("want nil, got %v", err)
		}
	})

	t.Run("EOF", func(t *testing.T) {
		h := NewTunHandler(context.Background(), reader([][]byte{nil}, []error{io.EOF}), &mockEncoder{}, &mockParser{}, &mockMgr{})
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF, got %v", err)
		}
	})

	t.Run("os.IsNotExist", func(t *testing.T) {
		perr := &os.PathError{Err: os.ErrNotExist}
		h := NewTunHandler(context.Background(), reader([][]byte{nil}, []error{perr}), &mockEncoder{}, &mockParser{}, &mockMgr{})
		if err := h.HandleTun(); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("want os.ErrNotExist, got %v", err)
		}
	})

	t.Run("os.IsPermission", func(t *testing.T) {
		perr := &os.PathError{Err: os.ErrPermission}
		h := NewTunHandler(context.Background(), reader([][]byte{nil}, []error{perr}), &mockEncoder{}, &mockParser{}, &mockMgr{})
		if err := h.HandleTun(); !errors.Is(err, os.ErrPermission) {
			t.Errorf("want os.ErrPermission, got %v", err)
		}
	})

	t.Run("read temporary error", func(t *testing.T) {
		h := NewTunHandler(context.Background(),
			reader([][]byte{{}}, []error{errors.New("tmp"), io.EOF}),
			&mockEncoder{}, &mockParser{}, &mockMgr{})
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF after retry, got %v", err)
		}
	})

	t.Run("invalid IP data", func(t *testing.T) {
		h := NewTunHandler(context.Background(), reader([][]byte{{1, 2, 3, 4}}, []error{nil, io.EOF}), &mockEncoder{}, &mockParser{}, &mockMgr{})
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF for invalid IP data, got %v", err)
		}
	})

	t.Run("parser error", func(t *testing.T) {
		h := NewTunHandler(context.Background(), reader([][]byte{buf}, []error{nil, io.EOF}), &mockEncoder{}, &mockParser{err: errors.New("bad parser")}, &mockMgr{})
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF for parser error, got %v", err)
		}
	})

	t.Run("session not found", func(t *testing.T) {
		h := NewTunHandler(context.Background(), reader([][]byte{buf}, []error{nil, io.EOF}), &mockEncoder{}, &mockParser{}, &mockMgr{getErr: errors.New("no sess")})
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF for session not found, got %v", err)
		}
	})

	t.Run("encrypt error", func(t *testing.T) {
		crypto := &mockCrypto{err: errors.New("enc fail")}
		h := NewTunHandler(context.Background(), reader([][]byte{buf}, []error{nil, io.EOF}), &mockEncoder{}, &mockParser{}, &mockMgr{sess: makeSession(&mockConn{}, crypto)})
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF for encrypt error, got %v", err)
		}
	})

	t.Run("encode error", func(t *testing.T) {
		enc := &mockEncoder{err: errors.New("encode fail")}
		h := NewTunHandler(context.Background(), reader([][]byte{buf}, []error{nil, io.EOF}), enc, &mockParser{}, &mockMgr{sess: makeSession(&mockConn{}, &mockCrypto{})})
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF for encode error, got %v", err)
		}
	})

	t.Run("conn write error", func(t *testing.T) {
		c := &mockConn{err: errors.New("write fail")}
		mgr := &mockMgr{sess: makeSession(c, &mockCrypto{})}
		h := NewTunHandler(context.Background(), reader([][]byte{buf}, []error{nil, io.EOF}), &mockEncoder{}, &mockParser{}, mgr)
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF for write error, got %v", err)
		}
		if atomic.LoadInt32(&mgr.deleted) != 1 {
			t.Errorf("expected Delete to be called")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		c := &mockConn{}
		enc := &mockEncoder{}
		mgr := &mockMgr{sess: makeSession(c, &mockCrypto{})}
		h := NewTunHandler(context.Background(), reader([][]byte{make([]byte, 8)}, []error{nil, io.EOF}), enc, &mockParser{}, mgr)
		if err := h.HandleTun(); err != io.EOF {
			t.Errorf("want EOF for happy path, got %v", err)
		}
		if atomic.LoadInt32(&enc.called) == 0 {
			t.Errorf("expected encoder.Encode to be called")
		}
		if atomic.LoadInt32(&c.called) == 0 {
			t.Errorf("expected conn.Write to be called")
		}
	})
}
