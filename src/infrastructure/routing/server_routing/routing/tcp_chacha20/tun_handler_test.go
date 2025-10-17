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
	"tungo/application/network/connection"
)

// --- Mocks (prefixed with the struct under test: TunHandler*) ---

// TunHandlerMockReader simulates a TUN reader with a scripted sequence.
type TunHandlerMockReader struct {
	seq [][]byte
	err []error
	i   int
}

func (r *TunHandlerMockReader) Read(p []byte) (int, error) {
	if r.i >= len(r.seq) {
		return 0, io.EOF
	}
	n := copy(p, r.seq[r.i])
	e := r.err[r.i]
	r.i++
	return n, e
}

// TunHandlerMockParser returns a preconfigured address or error.
type TunHandlerMockParser struct {
	addr netip.Addr
	err  error
}

func (p *TunHandlerMockParser) DestinationAddress(_ []byte) (netip.Addr, error) {
	return p.addr, p.err
}

// TunHandlerMockCrypto simulates Encrypt/Decrypt.
type TunHandlerMockCrypto struct{ err error }

func (m *TunHandlerMockCrypto) Encrypt(b []byte) ([]byte, error) {
	return append([]byte(nil), b...), m.err
}
func (m *TunHandlerMockCrypto) Decrypt([]byte) ([]byte, error) { return nil, nil }

// TunHandlerMockConn counts Write calls and can fail.
type TunHandlerMockConn struct {
	called int32
	err    error
}

func (c *TunHandlerMockConn) Write(b []byte) (int, error) {
	atomic.AddInt32(&c.called, 1)
	return len(b), c.err
}
func (c *TunHandlerMockConn) Read([]byte) (int, error)           { return 0, nil }
func (c *TunHandlerMockConn) Close() error                       { return nil }
func (c *TunHandlerMockConn) LocalAddr() net.Addr                { return nil }
func (c *TunHandlerMockConn) RemoteAddr() net.Addr               { return nil }
func (c *TunHandlerMockConn) SetDeadline(_ time.Time) error      { return nil }
func (c *TunHandlerMockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *TunHandlerMockConn) SetWriteDeadline(_ time.Time) error { return nil }

// TunHandlerMockMgr is a fake session repository.
type TunHandlerMockMgr struct {
	sess    Session
	getErr  error
	deleted int32
}

func (m *TunHandlerMockMgr) Add(_ connection.Session)    {}
func (m *TunHandlerMockMgr) Delete(_ connection.Session) { atomic.AddInt32(&m.deleted, 1) }
func (m *TunHandlerMockMgr) GetByInternalAddrPort(_ netip.Addr) (connection.Session, error) {
	return m.sess, m.getErr
}
func (m *TunHandlerMockMgr) GetByExternalAddrPort(_ netip.AddrPort) (connection.Session, error) {
	return m.sess, nil
}

// helper to build a session for tests (assumes local Session type in package)
func makeSession(c *TunHandlerMockConn, crypto *TunHandlerMockCrypto) Session {
	in, _ := netip.ParseAddr("10.0.0.1")
	ex, _ := netip.ParseAddrPort("203.0.113.1:9000")
	return Session{
		connectionAdapter:   c,
		cryptographyService: crypto,
		internalIP:          in,
		externalIP:          ex,
	}
}

func rdr(seq [][]byte, err []error) io.Reader { return &TunHandlerMockReader{seq: seq, err: err} }

// TunHandlerTempNetError simulates a temporary read error.
type TunHandlerTempNetError struct{}

func (TunHandlerTempNetError) Error() string   { return "temporary error" }
func (TunHandlerTempNetError) Temporary() bool { return true }
func (TunHandlerTempNetError) Timeout() bool   { return false }

// --- Tests aiming for 100% coverage of HandleTun ---

func TestTunHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewTunHandler(ctx, rdr(nil, nil), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
	if err := h.HandleTun(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTunHandler_EOF(t *testing.T) {
	h := NewTunHandler(context.Background(), rdr([][]byte{nil}, []error{io.EOF}), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestTunHandler_ReadErrors(t *testing.T) {
	t.Run("not exist", func(t *testing.T) {
		perr := &os.PathError{Err: os.ErrNotExist}
		h := NewTunHandler(context.Background(), rdr([][]byte{nil}, []error{perr}), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
		if err := h.HandleTun(); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("want os.ErrNotExist, got %v", err)
		}
	})
	t.Run("permission", func(t *testing.T) {
		perr := &os.PathError{Err: os.ErrPermission}
		h := NewTunHandler(context.Background(), rdr([][]byte{nil}, []error{perr}), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
		if err := h.HandleTun(); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("want os.ErrPermission, got %v", err)
		}
	})
	t.Run("temporary then EOF", func(t *testing.T) {
		h := NewTunHandler(context.Background(), rdr([][]byte{{1}}, []error{errors.New("tmp")}), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
		if err := h.HandleTun(); err == nil || err.Error() != "tmp" {
			t.Fatalf("want tmp error, got %v", err)
		}
	})
}

func TestTunHandler_ParserError(t *testing.T) {
	p := &TunHandlerMockParser{err: errors.New("bad header")}
	h := NewTunHandler(context.Background(), rdr([][]byte{{1, 2, 3, 4}}, []error{nil, io.EOF}), p, &TunHandlerMockMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestTunHandler_SessionNotFound(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.2")
	p := &TunHandlerMockParser{addr: addr}
	h := NewTunHandler(context.Background(), rdr([][]byte{{1, 2, 3, 4}}, []error{nil, io.EOF}), p, &TunHandlerMockMgr{getErr: errors.New("no sess")})
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestTunHandler_EncryptError(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.3")
	p := &TunHandlerMockParser{addr: addr}
	crypto := &TunHandlerMockCrypto{err: errors.New("enc fail")}
	mgr := &TunHandlerMockMgr{sess: makeSession(&TunHandlerMockConn{}, crypto)}
	h := NewTunHandler(context.Background(), rdr([][]byte{{9, 9, 9}}, []error{nil, io.EOF}), p, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestTunHandler_WriteErrorDeletesSession(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.4")
	p := &TunHandlerMockParser{addr: addr}
	c := &TunHandlerMockConn{err: errors.New("write fail")}
	mgr := &TunHandlerMockMgr{sess: makeSession(c, &TunHandlerMockCrypto{})}
	h := NewTunHandler(context.Background(), rdr([][]byte{{1, 2}}, []error{nil, io.EOF}), p, mgr)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	if atomic.LoadInt32(&mgr.deleted) != 1 {
		t.Fatalf("expected Delete to be called once")
	}
}

func TestTunHandler_HappyPath_V4(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.5")
	p := &TunHandlerMockParser{addr: addr}
	c := &TunHandlerMockConn{}
	mgr := &TunHandlerMockMgr{sess: makeSession(c, &TunHandlerMockCrypto{})}
	h := NewTunHandler(context.Background(), rdr([][]byte{make([]byte, 8)}, []error{nil, io.EOF}), p, mgr)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	if atomic.LoadInt32(&c.called) == 0 {
		t.Fatalf("expected connectionAdapter.Write to be called")
	}
}

func TestTunHandler_HappyPath_V6(t *testing.T) {
	addr := netip.MustParseAddr("2001:db8::1")
	p := &TunHandlerMockParser{addr: addr}
	c := &TunHandlerMockConn{}
	mgr := &TunHandlerMockMgr{sess: makeSession(c, &TunHandlerMockCrypto{})}
	h := NewTunHandler(context.Background(), rdr([][]byte{make([]byte, 12)}, []error{nil, io.EOF}), p, mgr)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	if atomic.LoadInt32(&c.called) == 0 {
		t.Fatalf("expected connectionAdapter.Write to be called")
	}
}

func TestTunHandler_ZeroLengthRead_SkipsThenEOF(t *testing.T) {
	h := NewTunHandler(
		context.Background(),
		rdr([][]byte{{}}, []error{nil, io.EOF}), // n==0, then EOF
		&TunHandlerMockParser{},                 // wil not be called
		&TunHandlerMockMgr{},
	)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF after zero-length read, got %v", err)
	}
}

func TestTunHandler_TemporaryReadError_RetriesThenEOF(t *testing.T) {
	// First Read: returns any bytes + temporary error -> handler must 'continue'
	// Second Read: mock returns EOF (because scripted seq is exhausted) -> handler returns EOF.
	h := NewTunHandler(
		context.Background(),
		rdr([][]byte{{0xAA}}, []error{TunHandlerTempNetError{}}),
		&TunHandlerMockParser{}, // not used
		&TunHandlerMockMgr{},    // not used
	)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF after temporary error retry, got %v", err)
	}
}

func TestTunHandler_TemporaryReadError_ThenProcessesNextPacket(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.42")
	p := &TunHandlerMockParser{addr: addr}
	conn := &TunHandlerMockConn{}
	mgr := &TunHandlerMockMgr{sess: makeSession(conn, &TunHandlerMockCrypto{})}

	// Read #1 -> temporary error; Read #2 -> valid payload; Read #3 -> EOF
	h := NewTunHandler(
		context.Background(),
		rdr([][]byte{{0x01}, {0xDE, 0xAD, 0xBE, 0xEF}}, []error{TunHandlerTempNetError{}, nil}),
		p,
		mgr,
	)

	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if got := atomic.LoadInt32(&conn.called); got == 0 {
		t.Fatalf("expected connection Write to be called after temporary error, got %d", got)
	}
}
