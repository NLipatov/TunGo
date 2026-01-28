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
	"tungo/infrastructure/cryptography/chacha20/rekey"

	"golang.org/x/crypto/chacha20poly1305"

	"tungo/application/network/connection"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/settings"
)

/* ========= fakes / test doubles ========= */

type fakeTcpListenerCtxDone struct {
	t      *testing.T
	ctx    context.Context
	closed bool
	called bool
}

func (f *fakeTcpListenerCtxDone) Accept() (net.Conn, error) {
	if !f.called {
		f.called = true
		<-f.ctx.Done()
		return nil, errors.New("ctx canceled")
	}
	return nil, errors.New("done")
}
func (f *fakeTcpListenerCtxDone) Close() error { f.closed = true; return nil }

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
func (f *fakeConn) Write(b []byte) (int, error)        { return f.writeBuf.Write(b) }
func (f *fakeConn) Close() error                       { f.closed = true; return nil }
func (f *fakeConn) LocalAddr() net.Addr                { return f.addr }
func (f *fakeConn) RemoteAddr() net.Addr               { return f.addr }
func (f *fakeConn) SetDeadline(_ time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(_ time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(_ time.Time) error { return nil }
func tcpAddr(ip string, port int) *net.TCPAddr {
	a, _ := net.ResolveTCPAddr("tcp", net.JoinHostPort(ip, fmt.Sprint(port)))
	return a
}
func mustAddrPort(s string) netip.AddrPort { return netip.MustParseAddrPort(s) }

type fakeTcpListener struct {
	conns       []net.Conn
	acceptIx    int
	err         error
	closed      bool
	errCount    int
	maxErrCount int
}

func (f *fakeTcpListener) Accept() (net.Conn, error) {
	if f.err != nil {
		f.errCount++
		if f.errCount >= f.maxErrCount {
			time.Sleep(30 * time.Millisecond)
		}
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

func (l *fakeLogger) Printf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprintf(format, args...))
}
func (l *fakeLogger) contains(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.logs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
func (l *fakeLogger) count(sub string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.logs {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

type fakeHandshake struct {
	ip     net.IP
	err    error
	id     [32]byte
	client [32]byte
	server [32]byte
}

func (f *fakeHandshake) Id() [32]byte              { return f.id }
func (f *fakeHandshake) KeyClientToServer() []byte { return f.client[:] }
func (f *fakeHandshake) KeyServerToClient() []byte { return f.server[:] }
func (f *fakeHandshake) ServerSideHandshake(_ connection.Transport) (net.IP, error) {
	return f.ip, f.err
}
func (f *fakeHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	return nil
}

type fakeHandshakeFactory struct{ hs connection.Handshake }

func (f *fakeHandshakeFactory) NewHandshake() connection.Handshake { return f.hs }

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

func (f *fakeCryptoFactory) FromHandshake(_ connection.Handshake, _ bool) (connection.Crypto, *rekey.Controller, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return &fakeCrypto{}, nil, nil
}

type fakeSessionRepo struct {
	sessions       map[netip.AddrPort]connection.Session
	added, deleted []connection.Session
	getErr         error
	returnSession  connection.Session
}

func (r *fakeSessionRepo) Add(s connection.Session) {
	if r.sessions == nil {
		r.sessions = make(map[netip.AddrPort]connection.Session)
	}
	r.sessions[s.ExternalAddrPort()] = s
	r.added = append(r.added, s)
}
func (r *fakeSessionRepo) Delete(s connection.Session) { r.deleted = append(r.deleted, s) }
func (r *fakeSessionRepo) GetByInternalAddrPort(_ netip.Addr) (connection.Session, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.returnSession, nil
}
func (r *fakeSessionRepo) GetByExternalAddrPort(addr netip.AddrPort) (connection.Session, error) {
	s, ok := r.sessions[addr]
	if !ok {
		return Session{}, errors.New("no session")
	}
	return s, nil
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

/* ========= tests ========= */

func TestHandleTransport_CtxDoneBeforeAccept_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener := &fakeTcpListenerCtxDone{ctx: ctx, t: t}
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.HandleTransport()
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	err := <-errCh

	if err != nil {
		t.Fatalf("expected nil error on ctx cancel during Accept, got %v", err)
	}
	if !listener.closed {
		t.Error("listener should be closed on ctx done")
	}
}

func TestHandleTransport_AlreadyCanceled_ReturnsCtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// listener: Accept should NOT be called in this path
	listener := &fakeTcpListener{conns: nil}
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

	err := handler.HandleTransport()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !listener.closed {
		t.Error("listener should be closed by defer")
	}
}

func TestHandleTransport_AcceptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	maxErrs := 3

	listener := &fakeTcpListener{err: errors.New("accept fail"), maxErrCount: maxErrs}
	logger := &fakeLogger{}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "1111"},
		&fakeWriter{},
		listener,
		&fakeSessionRepo{},
		logger,
		&fakeHandshakeFactory{},
		&fakeCryptoFactory{},
	)

	done := make(chan struct{})
	go func() {
		_ = handler.HandleTransport()
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	count := logger.count("failed to accept connection: accept fail")
	if count != maxErrs {
		t.Errorf("expected %d error logs, got %d (logs=%v)", maxErrs, count, logger.logs)
	}
}

func TestHandleTransport_RegisterClientError_Logged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := tcpAddr("127.0.0.1", 8888)
	fconn := &fakeConn{addr: addr}
	listener := &fakeTcpListener{conns: []net.Conn{fconn}}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}, err: errors.New("handshake fail")}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}

	handler := NewTransportHandler(ctx, settings.Settings{Port: "2222"}, &fakeWriter{}, listener, &fakeSessionRepo{}, logger, handshakeFactory, &fakeCryptoFactory{})
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if !logger.contains("failed to register client") {
		t.Errorf("expected a 'failed to register client' log, got: %v", logger.logs)
	}
}

func TestRegisterClient_HandshakeError(t *testing.T) {
	ctx := context.Background()
	addr := tcpAddr("127.0.0.1", 9991)
	fconn := &fakeConn{addr: addr}
	w := &fakeWriter{}
	logger := &fakeLogger{}
	hs := &fakeHandshake{ip: []byte{127, 0, 0, 1}, err: errors.New("boom")}
	hf := &fakeHandshakeFactory{hs: hs}

	h := NewTransportHandler(ctx, settings.Settings{}, w, &fakeTcpListener{}, &fakeSessionRepo{}, logger, hf, &fakeCryptoFactory{})
	err := h.(*TransportHandler).registerClient(fconn, w, ctx)
	if err == nil || !strings.Contains(err.Error(), "client 127.0.0.1:9991 failed registration: boom") {
		t.Fatalf("expected handshake error, got %v", err)
	}
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

type badAddrConn struct{ fakeConn }

func (c *badAddrConn) RemoteAddr() net.Addr { return &struct{ net.Addr }{} }

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

	oldSession := Session{externalIP: mustAddrPort("127.0.0.1:8070")}
	srepo := &fakeSessionRepo{
		getErr:        nil,
		returnSession: oldSession, // existing session -> will be deleted
	}
	handler := NewTransportHandler(ctx, settings.Settings{}, writer, &fakeTcpListener{}, srepo, logger, handshakeFactory, &fakeCryptoFactory{})

	_ = handler.(*TransportHandler).registerClient(fconn, writer, ctx)

	// We expect exactly two Delete calls:
	// 1) old replaced session, 2) new session in handleClient's defer
	if len(srepo.deleted) != 2 {
		t.Fatalf("expected 2 Delete calls (old + new), got %d: %+v", len(srepo.deleted), srepo.deleted)
	}
	for i, del := range srepo.deleted {
		if del.ExternalAddrPort().Addr().Unmap() != oldSession.ExternalAddrPort().Addr().Unmap() ||
			del.ExternalAddrPort().Port() != oldSession.ExternalAddrPort().Port() {
			t.Errorf("deleted session #%d has wrong externalIP: %v, want %v", i+1, del.ExternalAddrPort(), oldSession.externalIP)
		}
	}
}

func TestRegisterClient_AddsSessionOnNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := tcpAddr("127.0.0.1", 6010)
	fconn := &fakeConn{addr: addr}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	handshake := &fakeHandshake{ip: []byte{127, 0, 0, 1}}
	handshakeFactory := &fakeHandshakeFactory{hs: handshake}
	repo := &fakeSessionRepo{getErr: repository.ErrSessionNotFound}

	h := NewTransportHandler(ctx, settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, handshakeFactory, &fakeCryptoFactory{})
	if err := h.(*TransportHandler).registerClient(fconn, writer, ctx); err != nil {
		t.Fatalf("registerClient returned error: %v", err)
	}
	if len(repo.added) != 1 {
		t.Fatalf("expected 1 added session, got %d", len(repo.added))
	}
	added := repo.added[0]
	if added.ExternalAddrPort().Addr().Unmap().String() != "127.0.0.1" {
		t.Errorf("added session addr mismatch: %v", added.ExternalAddrPort())
	}
}

func TestHandleClient_CtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &fakeConn{addr: tcpAddr("1.2.3.4", 5555)}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{connectionAdapter: conn, cryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.5"), externalIP: mustAddrPort("1.2.3.4:5555")}
	repo := &fakeSessionRepo{}

	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)

	if len(repo.deleted) != 1 {
		t.Fatalf("expected session deleted once on ctx cancel, got %d", len(repo.deleted))
	}
	if !conn.closed {
		t.Fatalf("expected transport Close() to be called")
	}
}

func TestHandleClient_ReadFullError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{addr: tcpAddr("1.2.3.4", 5556), readErr: errors.New("fail read")}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{connectionAdapter: conn, cryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.6"), externalIP: mustAddrPort("1.2.3.4:5556")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)

	if !logger.contains("failed to read from client: fail read") {
		t.Errorf("expected read error log, got %v", logger.logs)
	}
	if len(repo.deleted) != 1 {
		t.Fatalf("expected session deleted once on read error, got %d", len(repo.deleted))
	}
	if !conn.closed {
		t.Fatalf("expected transport Close() to be called")
	}
}

func TestHandleClient_EOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{addr: tcpAddr("1.2.3.4", 5557)}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{connectionAdapter: conn, cryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.7"), externalIP: mustAddrPort("1.2.3.4:5557")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)

	if len(repo.deleted) != 1 {
		t.Fatalf("expected session deleted once on EOF, got %d", len(repo.deleted))
	}
	if !conn.closed {
		t.Fatalf("expected transport Close() to be called")
	}
}

func TestHandleClient_BadLength(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// shorter than AEAD overhead
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5558),
		readBufs: [][]byte{make([]byte, chacha20poly1305.Overhead-1)},
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{connectionAdapter: conn, cryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.8"), externalIP: mustAddrPort("1.2.3.4:5558")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)

	if !logger.contains("invalid ciphertext length:") {
		t.Errorf("expected invalid length log, got %v", logger.logs)
	}
}

func TestHandleClient_DecryptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5560),
		readBufs: [][]byte{make([]byte, chacha20poly1305.Overhead)}, // valid length
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{connectionAdapter: conn, cryptographyService: &fakeCrypto{decErr: errors.New("bad decrypt")}, internalIP: netip.MustParseAddr("10.0.0.10"), externalIP: mustAddrPort("1.2.3.4:5560")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)

	if !logger.contains("failed to decrypt data: bad decrypt") {
		t.Errorf("expected decrypt error log, got %v", logger.logs)
	}
}

func TestHandleClient_WriteTunError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5561),
		readBufs: [][]byte{make([]byte, chacha20poly1305.Overhead)}, // valid length
	}
	writer := &fakeWriter{err: errors.New("fail tun")}
	logger := &fakeLogger{}
	sess := Session{connectionAdapter: conn, cryptographyService: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.11"), externalIP: mustAddrPort("1.2.3.4:5561")}
	repo := &fakeSessionRepo{}
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})
	handler.(*TransportHandler).handleClient(ctx, sess, writer)

	if !logger.contains("failed to write to TUN: fail tun") {
		t.Errorf("expected write-to-tun error log, got %v", logger.logs)
	}
}

func TestHandleClient_HappyDataPath_WritesToTun_AndCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	payload := make([]byte, chacha20poly1305.Overhead) // minimal valid length

	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 6001),
		readBufs: [][]byte{payload},
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := Session{
		connectionAdapter:   conn,
		cryptographyService: &fakeCrypto{},
		internalIP:          netip.MustParseAddr("10.0.0.12"),
		externalIP:          mustAddrPort("1.2.3.4:6001"),
	}
	repo := &fakeSessionRepo{}
	h := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{})

	h.(*TransportHandler).handleClient(ctx, sess, writer)

	if len(writer.wrote) != 1 {
		t.Fatalf("expected 1 write to TUN, got %d", len(writer.wrote))
	}
	if !bytes.Equal(writer.wrote[0], payload) {
		t.Fatalf("written payload mismatch: got %v, want %v", writer.wrote[0], payload)
	}
	if len(repo.deleted) != 1 {
		t.Fatalf("expected session deleted once on normal return, got %d", len(repo.deleted))
	}
	if !conn.closed {
		t.Fatalf("expected transport Close() to be called")
	}
}
