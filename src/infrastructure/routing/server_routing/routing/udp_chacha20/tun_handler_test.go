package udp_chacha20

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
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// ---------- Mocks (prefixed with the struct under test: TunHandler*) ----------

type TunHandlerMockReader struct {
	seq []struct {
		data []byte
		err  error
	}
	i int
}

func (r *TunHandlerMockReader) Read(p []byte) (int, error) {
	if r.i >= len(r.seq) {
		return 0, io.EOF
	}
	rec := r.seq[r.i]
	r.i++
	if rec.data == nil {
		return 0, rec.err
	}
	n := copy(p, rec.data)
	return n, rec.err
}

// HeaderParser mock for the new interface.
type TunHandlerMockParser struct {
	addr netip.Addr
	err  error
}

func (p *TunHandlerMockParser) DestinationAddress(_ []byte) (netip.Addr, error) {
	return p.addr, p.err
}

// crypto mock.
// Assumes Encrypt returns a new slice (ct).
type TunHandlerMockCrypto struct{ err error }

func (m *TunHandlerMockCrypto) Encrypt(b []byte) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	// return a copy to simulate "new" ciphertext
	return append([]byte(nil), b...), nil
}
func (m *TunHandlerMockCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

// net.Conn mock (only Write is observed).
type TunHandlerMockConn struct {
	writes int32
	err    error
}

func (c *TunHandlerMockConn) Write(b []byte) (int, error) {
	atomic.AddInt32(&c.writes, 1)
	return len(b), c.err
}
func (c *TunHandlerMockConn) Read([]byte) (int, error)           { return 0, io.EOF }
func (c *TunHandlerMockConn) Close() error                       { return nil }
func (c *TunHandlerMockConn) LocalAddr() net.Addr                { return nil }
func (c *TunHandlerMockConn) RemoteAddr() net.Addr               { return nil }
func (c *TunHandlerMockConn) SetDeadline(_ time.Time) error      { return nil }
func (c *TunHandlerMockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *TunHandlerMockConn) SetWriteDeadline(_ time.Time) error { return nil }

// mockSession implements connection.Session (minimal methods used by handler).
// mockSession implements connection.Session (minimal methods used by handler).
type mockSession struct {
	transport net.Conn
	crypto    *TunHandlerMockCrypto
	internal  netip.Addr
	external  netip.AddrPort
}

func (m *mockSession) Crypto() connection.Crypto        { return m.crypto }
func (m *mockSession) Transport() connection.Transport  { return m.transport }
func (m *mockSession) ExternalAddrPort() netip.AddrPort { return m.external }
func (m *mockSession) InternalAddr() netip.Addr         { return m.internal }
func (m *mockSession) Outbound() connection.Outbound {
	return connection.NewDefaultOutbound(m.transport, m.crypto)
}
func (m *mockSession) RekeyController() rekey.FSM { return nil }

// helper to build a session that matches the handler expectations
func mkSession(c *TunHandlerMockConn, crypto *TunHandlerMockCrypto) connection.Session {
	in := netip.MustParseAddr("10.0.0.1")
	ex := netip.MustParseAddrPort("203.0.113.1:9000")
	return &mockSession{
		transport: c,
		crypto:    crypto,
		internal:  in,
		external:  ex,
	}
}

// Session repo mock.
type TunHandlerMockMgr struct {
	sess    connection.Session
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

func rdr(seq ...struct {
	data []byte
	err  error
}) io.Reader {
	return &TunHandlerMockReader{seq: seq}
}

// ------------------------------- Tests ---------------------------------------

func TestTunHandler_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewTunHandler(ctx, rdr(), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
	if err := h.HandleTun(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestTunHandler_EOF(t *testing.T) {
	r := rdr(struct {
		data []byte
		err  error
	}{data: make([]byte, 20), err: io.EOF})
	h := NewTunHandler(context.Background(), r, &TunHandlerMockParser{}, &TunHandlerMockMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

func TestTunHandler_ReadOsErrors(t *testing.T) {
	t.Run("not exist", func(t *testing.T) {
		perr := &os.PathError{Op: "read", Path: "/dev/net/tun", Err: os.ErrNotExist}
		h := NewTunHandler(context.Background(), rdr(struct {
			data []byte
			err  error
		}{nil, perr}), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
		if err := h.HandleTun(); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("want os.ErrNotExist, got %v", err)
		}
	})
	t.Run("permission", func(t *testing.T) {
		perr := &os.PathError{Op: "read", Path: "/dev/net/tun", Err: os.ErrPermission}
		h := NewTunHandler(context.Background(), rdr(struct {
			data []byte
			err  error
		}{nil, perr}), &TunHandlerMockParser{}, &TunHandlerMockMgr{})
		if err := h.HandleTun(); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("want os.ErrPermission, got %v", err)
		}
	})
}

func TestTunHandler_TemporaryThenEOF(t *testing.T) {
	r := rdr(
		struct {
			data []byte
			err  error
		}{nil, errors.New("tmp read")},
		struct {
			data []byte
			err  error
		}{nil, io.EOF},
	)
	h := NewTunHandler(context.Background(), r, &TunHandlerMockParser{}, &TunHandlerMockMgr{})
	// In current handler implementation a non-temporary error will be returned as-is.
	// Our mock returns a non-Temporary error "tmp read", so HandleTun should return it.
	if err := h.HandleTun(); err == nil || err.Error() != "tmp read" {
		t.Fatalf("want tmp read after retry, got %v", err)
	}
}

func TestTunHandler_ZeroLengthRead_Skips(t *testing.T) {
	r := rdr(
		struct {
			data []byte
			err  error
		}{[]byte{}, nil}, // n==0 path
		struct {
			data []byte
			err  error
		}{nil, io.EOF},
	)
	h := NewTunHandler(context.Background(), r, &TunHandlerMockParser{}, &TunHandlerMockMgr{})
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
}

func TestTunHandler_ParserError(t *testing.T) {
	ip4 := make([]byte, 20)
	ip4[0] = 0x45
	p := &TunHandlerMockParser{err: errors.New("bad header")}
	r := rdr(
		struct {
			data []byte
			err  error
		}{ip4, nil}, // give pure IP header (no nonce)
		struct {
			data []byte
			err  error
		}{nil, io.EOF},
	)
	a := &TunHandlerMockConn{}
	mgr := &TunHandlerMockMgr{sess: mkSession(a, &TunHandlerMockCrypto{})}
	h := NewTunHandler(context.Background(), r, p, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Fatalf("writes=%d, want 0", n)
	}
}

func TestTunHandler_SessionNotFound(t *testing.T) {
	ip4 := make([]byte, 20)
	ip4[0] = 0x45
	dst := netip.MustParseAddr("10.0.0.1")
	p := &TunHandlerMockParser{addr: dst}
	r := rdr(struct {
		data []byte
		err  error
	}{ip4, nil}, struct {
		data []byte
		err  error
	}{nil, io.EOF})
	a := &TunHandlerMockConn{}
	mgr := &TunHandlerMockMgr{sess: mkSession(a, &TunHandlerMockCrypto{}), getErr: errors.New("no sess")}
	h := NewTunHandler(context.Background(), r, p, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Fatalf("writes=%d, want 0", n)
	}
}

func TestTunHandler_EncryptError(t *testing.T) {
	ip4 := make([]byte, 20)
	ip4[0] = 0x45
	dst := netip.MustParseAddr("10.0.0.2")
	p := &TunHandlerMockParser{addr: dst}
	r := rdr(struct {
		data []byte
		err  error
	}{ip4, nil}, struct {
		data []byte
		err  error
	}{nil, io.EOF})
	a := &TunHandlerMockConn{}
	crypto := &TunHandlerMockCrypto{err: errors.New("enc fail")}
	mgr := &TunHandlerMockMgr{sess: mkSession(a, crypto)}
	h := NewTunHandler(context.Background(), r, p, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 0 {
		t.Fatalf("writes=%d, want 0", n)
	}
}

func TestTunHandler_WriteError_NoDelete(t *testing.T) {
	ip4 := make([]byte, 20)
	ip4[0] = 0x45
	dst := netip.MustParseAddr("10.0.0.3")
	p := &TunHandlerMockParser{addr: dst}
	r := rdr(struct {
		data []byte
		err  error
	}{ip4, nil}, struct {
		data []byte
		err  error
	}{nil, io.EOF})
	a := &TunHandlerMockConn{err: errors.New("write fail")}
	mgr := &TunHandlerMockMgr{sess: mkSession(a, &TunHandlerMockCrypto{})}
	h := NewTunHandler(context.Background(), r, p, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 1 {
		t.Fatalf("writes=%d, want 1", n)
	}
	// UDP branch: no Delete() on single write error
	if atomic.LoadInt32(&mgr.deleted) != 0 {
		t.Fatalf("Delete() should not be called for UDP write error")
	}
}

func TestTunHandler_Happy_V4(t *testing.T) {
	ip4 := make([]byte, 20)
	ip4[0] = 0x45
	dst := netip.MustParseAddr("10.0.0.4")
	p := &TunHandlerMockParser{addr: dst}
	r := rdr(struct {
		data []byte
		err  error
	}{ip4, nil}, struct {
		data []byte
		err  error
	}{nil, io.EOF})
	a := &TunHandlerMockConn{}
	mgr := &TunHandlerMockMgr{sess: mkSession(a, &TunHandlerMockCrypto{})}
	h := NewTunHandler(context.Background(), r, p, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 1 {
		t.Fatalf("writes=%d, want 1", n)
	}
}

func TestTunHandler_Happy_V6(t *testing.T) {
	ip6 := make([]byte, 40)
	ip6[0] = 0x60
	dst := netip.MustParseAddr("2001:db8::1")
	p := &TunHandlerMockParser{addr: dst}
	r := rdr(struct {
		data []byte
		err  error
	}{ip6, nil}, struct {
		data []byte
		err  error
	}{nil, io.EOF})
	a := &TunHandlerMockConn{}
	mgr := &TunHandlerMockMgr{sess: mkSession(a, &TunHandlerMockCrypto{})}
	h := NewTunHandler(context.Background(), r, p, mgr)
	if err := h.HandleTun(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	if n := atomic.LoadInt32(&a.writes); n != 1 {
		t.Fatalf("writes=%d, want 1", n)
	}
}
