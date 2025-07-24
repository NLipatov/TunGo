package tcp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

type fakeConn struct {
	readBufs [][]byte
	writeBuf bytes.Buffer
	readIdx  int
	closed   bool
	addr     *net.TCPAddr
	readErr  error
}

func (f *fakeConn) Read(b []byte) (int, error) {
	if f.readIdx >= len(f.readBufs) {
		if f.readErr != nil {
			return 0, f.readErr
		}
		return 0, io.EOF
	}

	n := copy(b, f.readBufs[f.readIdx])

	if n < len(f.readBufs[f.readIdx]) {
		f.readBufs[f.readIdx] = f.readBufs[f.readIdx][n:]
	} else {
		f.readIdx++
	}

	return n, nil
}

func (f *fakeConn) Write(b []byte) (int, error) {
	return f.writeBuf.Write(b)
}
func (f *fakeConn) Close() error                       { f.closed = true; return nil }
func (f *fakeConn) LocalAddr() net.Addr                { return f.addr }
func (f *fakeConn) RemoteAddr() net.Addr               { return f.addr }
func (f *fakeConn) SetDeadline(_ time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(_ time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(_ time.Time) error { return nil }

type fakeTcpListener struct {
	conns    []net.Conn
	acceptIx int
	err      error
	closed   bool
}

func (f *fakeTcpListener) Accept() (net.Conn, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.acceptIx >= len(f.conns) {
		time.Sleep(10 * time.Millisecond)
		return nil, io.EOF
	}
	c := f.conns[f.acceptIx]
	f.acceptIx++
	return c, nil
}
func (f *fakeTcpListener) Close() error { f.closed = true; return nil }

type fakeLogger struct {
	logs []string
	mu   sync.Mutex
}

func (l *fakeLogger) Printf(format string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, format)
}

type fakeHandshake struct {
	ip     net.IP
	err    error
	id     [32]byte
	client [32]byte
	server [32]byte
}

func (f *fakeHandshake) Id() [32]byte      { return f.id }
func (f *fakeHandshake) ClientKey() []byte { return f.client[:] }
func (f *fakeHandshake) ServerKey() []byte { return f.server[:] }
func (f *fakeHandshake) ServerSideHandshake(_ application.ConnectionAdapter) (net.IP, error) {
	return f.ip, f.err
}
func (f *fakeHandshake) ClientSideHandshake(_ application.ConnectionAdapter, _ settings.Settings) error {
	return nil
}

type fakeHandshakeFactory struct{ hs application.Handshake }

func (f *fakeHandshakeFactory) NewHandshake() application.Handshake { return f.hs }

type fakeCrypto struct {
	decErr error
}

func (f *fakeCrypto) Encrypt(in []byte) ([]byte, error) { return in, nil }
func (f *fakeCrypto) Decrypt(in []byte) ([]byte, error) {
	if f.decErr != nil {
		return nil, f.decErr
	}
	return in, nil
}

type fakeCryptoFactory struct{ err error }

func (f *fakeCryptoFactory) FromHandshake(_ application.Handshake, _ bool) (application.CryptographyService, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &fakeCrypto{}, nil
}

type fakeSessionRepo struct {
	sessions       map[netip.AddrPort]Session
	added, deleted []Session
	getErr         error
	returnSession  Session
}

func (r *fakeSessionRepo) Add(s Session) {
	if r.sessions == nil {
		r.sessions = make(map[netip.AddrPort]Session)
	}
	r.sessions[s.externalIP] = s
	r.added = append(r.added, s)
}
func (r *fakeSessionRepo) Delete(s Session) { r.deleted = append(r.deleted, s) }
func (r *fakeSessionRepo) GetByInternalAddrPort(_ netip.Addr) (Session, error) {
	if r.getErr != nil {
		return Session{}, r.getErr
	}
	return r.returnSession, nil
}
func (r *fakeSessionRepo) GetByExternalAddrPort(addr netip.AddrPort) (Session, error) {
	s, ok := r.sessions[addr]
	if !ok {
		return Session{}, errors.New("no session")
	}
	return s, nil
}

func (r *fakeSessionRepo) Range(f func(s Session) bool) {
	for _, s := range r.sessions {
		if !f(s) {
			break
		}
	}
}

type fakeWriter struct {
	buf   bytes.Buffer
	err   error
	wrote [][]byte
}

func (f *fakeWriter) Write(p []byte) (int, error) {
	f.wrote = append(f.wrote, append([]byte(nil), p...))
	if f.err != nil {
		return 0, f.err
	}
	return f.buf.Write(p)
}
func (f *fakeWriter) Read(_ []byte) (int, error) { return 0, io.EOF }
func (f *fakeWriter) Close() error               { return nil }

func tcpAddr(ip string, port int) *net.TCPAddr {
	addr, _ := net.ResolveTCPAddr("tcp", net.JoinHostPort(ip, fmt.Sprint(port)))
	return addr
}

func TestHandleTransport_CtxDoneBeforeAccept(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	listener := &fakeTcpListener{}
	logger := &fakeLogger{}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "7777"},
		&fakeWriter{},
		listener,
		&fakeSessionRepo{},
		logger,
		&fakeHandshakeFactory{},
		&fakeCryptoFactory{},
	)
	_ = handler.HandleTransport()
	if !listener.closed {
		t.Error("listener should be closed on ctx done")
	}
}

func TestHandleTransport_AcceptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener := &fakeTcpListener{err: errors.New("accept fail")}
	logger := &fakeLogger{}
	handler := NewTransportHandler(ctx, settings.Settings{Port: "1111"}, &fakeWriter{}, listener, &fakeSessionRepo{}, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	go func() { _ = handler.HandleTransport() }()
	time.Sleep(15 * time.Millisecond)
	cancel()
}

func TestHandleTransport_RegisterClientError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := tcpAddr("127.0.0.1", 8888)
	fconn := &fakeConn{addr: addr}
	listener := &fakeTcpListener{conns: []net.Conn{fconn}}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}, err: errors.New("handshake fail")}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}
	handler := NewTransportHandler(ctx, settings.Settings{Port: "2222"}, &fakeWriter{}, listener, &fakeSessionRepo{}, logger, handshakeFactory, &fakeCryptoFactory{})
	go func() { _ = handler.HandleTransport() }()
	time.Sleep(10 * time.Millisecond)
	cancel()
}

func TestRegisterClient_CryptoFactoryError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := tcpAddr("127.0.0.1", 9999)
	fconn := &fakeConn{addr: addr}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}
	handler := NewTransportHandler(ctx, settings.Settings{Port: "3333"}, writer, &fakeTcpListener{}, &fakeSessionRepo{}, logger, handshakeFactory, &fakeCryptoFactory{err: errors.New("crypto fail")})
	err := handler.(*TransportHandler).registerClient(fconn, writer, ctx)
	if err == nil || err.Error() != "client 127.0.0.1:9999 failed registration: crypto fail" {
		t.Errorf("expected crypto error, got %v", err)
	}
}

type badAddrConn struct {
	fakeConn
}

func (c *badAddrConn) RemoteAddr() net.Addr {
	return &struct{ net.Addr }{}
}

func TestRegisterClient_BadAddrType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fconn := &badAddrConn{fakeConn{addr: nil}}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}
	handler := NewTransportHandler(ctx, settings.Settings{}, writer, &fakeTcpListener{}, &fakeSessionRepo{}, logger, handshakeFactory, &fakeCryptoFactory{})
	err := handler.(*TransportHandler).registerClient(fconn, writer, ctx)
	if err == nil || !strings.HasPrefix(err.Error(), "invalid remote address type") {
		t.Errorf("expected 'invalid remote address type' error, got '%v'", err)
	}
}

func TestRegisterClient_BadInternalIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := tcpAddr("127.0.0.1", 8080)
	fconn := &fakeConn{addr: addr}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{1, 2}}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}
	handler := NewTransportHandler(ctx, settings.Settings{}, writer, &fakeTcpListener{}, &fakeSessionRepo{}, logger, handshakeFactory, &fakeCryptoFactory{})
	err := handler.(*TransportHandler).registerClient(fconn, writer, ctx)
	if err == nil || err.Error() != "invalid internal IP from handshake" {
		t.Errorf("expected bad internal IP error, got %v", err)
	}
}

func TestRegisterClient_SessionRepoGetError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := tcpAddr("127.0.0.1", 9090)
	fconn := &fakeConn{addr: addr}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}
	repo := &fakeSessionRepo{getErr: errors.New("db down")}
	handler := NewTransportHandler(ctx, settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, handshakeFactory, &fakeCryptoFactory{})
	err := handler.(*TransportHandler).registerClient(fconn, writer, ctx)
	if err == nil || !strings.HasPrefix(err.Error(), "connection closed") {
		t.Errorf("expected 'connection closed', got '%v'", err)
	}
}

func TestRegisterClient_ReplaceSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := tcpAddr("127.0.0.1", 8070)
	fconn := &fakeConn{addr: addr}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}

	oldSession := Session{externalIP: netip.MustParseAddrPort("127.0.0.1:8070")}
	srepo := &fakeSessionRepo{
		getErr:        nil,
		returnSession: oldSession, // will be returned as "existing session"
	}
	handler := NewTransportHandler(ctx, settings.Settings{}, writer, &fakeTcpListener{}, srepo, logger, handshakeFactory, &fakeCryptoFactory{})

	_ = handler.(*TransportHandler).registerClient(fconn, writer, ctx)

	// Strict check: we expect exactly two Delete calls, both for the same externalIP
	// The first Delete is for the replaced (old) session,
	// The second Delete is for the newly registered session when its goroutine finishes (via handleClient defer).
	// This is correct, because both represent the same connection (rebind to the same IP/port).
	if len(srepo.deleted) != 2 {
		t.Fatalf("expected 2 Delete calls (one for old, one for new session), got %d: %+v", len(srepo.deleted), srepo.deleted)
	}
	for i, del := range srepo.deleted {
		if del.externalIP.Addr().Unmap() != oldSession.externalIP.Addr().Unmap() ||
			del.externalIP.Port() != oldSession.externalIP.Port() {
			t.Errorf("deleted session #%d has wrong externalIP: %v, want %v", i+1, del.externalIP, oldSession.externalIP)
		}
	}
	/*
		Why 2 deletes?
		1. The first Delete is invoked when an existing session with the same internal IP is detected and replaced.
		2. The second Delete is called in handleClient's defer, which always deletes the current session when the client is closed.
		Both deletes refer to the same IP/port, which is expected when a client reconnects (for example, after NAT rebinding).
	*/
}

func TestRegisterClient_HappyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := tcpAddr("127.0.0.1", 6070)
	fconn := &fakeConn{addr: addr}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}
	srepo := &fakeSessionRepo{getErr: repository.ErrSessionNotFound}
	handler := NewTransportHandler(ctx, settings.Settings{}, writer, &fakeTcpListener{}, srepo, logger, handshakeFactory, &fakeCryptoFactory{})
	go func() {
		_ = handler.(*TransportHandler).registerClient(fconn, writer, ctx)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
}

func TestHandleClient_CtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &fakeConn{addr: tcpAddr("1.2.3.4", 5555)}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{conn: conn, CryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.5")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)
}

func TestHandleClient_ReadFullError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{addr: tcpAddr("1.2.3.4", 5556), readErr: errors.New("fail read")}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{conn: conn, CryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.6")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)
}

func TestHandleClient_EOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{addr: tcpAddr("1.2.3.4", 5557)}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{conn: conn, CryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.7")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)
}

func TestHandleClient_BadLength(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5558),
		readBufs: [][]byte{{0, 0, 0, 3}}, // length < 4
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{conn: conn, CryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.8")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)
}

func TestHandleClient_ReadPacketError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5559),
		readBufs: [][]byte{{0, 0, 0, 8}},
		readErr:  errors.New("fail to read packet"),
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{conn: conn, CryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.9")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)
}

func TestHandleClient_DecryptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5560),
		readBufs: [][]byte{{0, 0, 0, 5}, {1, 2, 3, 4, 5}},
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{conn: conn, CryptographyService: &fakeCrypto{decErr: errors.New("bad decrypt")}, internalIP: netip.MustParseAddr("10.0.0.10")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)
}

func TestHandleClient_WriteTunError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5561),
		readBufs: [][]byte{{0, 0, 0, 5}, {1, 2, 3, 4, 5}},
	}
	writer := &fakeWriter{err: errors.New("fail tun")}
	logger := &fakeLogger{}
	sess := Session{conn: conn, CryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.11")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)
}
