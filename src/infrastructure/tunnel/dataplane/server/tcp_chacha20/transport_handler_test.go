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

	"golang.org/x/crypto/chacha20poly1305"

	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/tcp_registration"
)

/* ========= fakes / test doubles ========= */

type noopEgress struct{}

func (noopEgress) SendDataIP([]byte) error  { return nil }
func (noopEgress) SendControl([]byte) error { return nil }
func (noopEgress) Close() error             { return nil }

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
	clientID int
	err         error
	id          [32]byte
	client      [32]byte
	server      [32]byte
}

func (f *fakeHandshake) Id() [32]byte              { return f.id }
func (f *fakeHandshake) KeyClientToServer() []byte { return f.client[:] }
func (f *fakeHandshake) KeyServerToClient() []byte { return f.server[:] }
func (f *fakeHandshake) ServerSideHandshake(_ connection.Transport) (int, error) {
	return f.clientID, f.err
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

func (f *fakeCryptoFactory) FromHandshake(_ connection.Handshake, _ bool) (connection.Crypto, *rekey.StateMachine, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return &fakeCrypto{}, nil, nil
}

type fakeSessionRepo struct {
	sessions   map[netip.AddrPort]*session.Peer
	added      []*session.Peer
	deleted    []*session.Peer
	getErr     error
	returnPeer *session.Peer
}

func (r *fakeSessionRepo) Add(p *session.Peer) {
	if r.sessions == nil {
		r.sessions = make(map[netip.AddrPort]*session.Peer)
	}
	r.sessions[p.ExternalAddrPort()] = p
	r.added = append(r.added, p)
}
func (r *fakeSessionRepo) Delete(p *session.Peer) { r.deleted = append(r.deleted, p) }
func (r *fakeSessionRepo) GetByInternalAddrPort(_ netip.Addr) (*session.Peer, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.returnPeer, nil
}
func (r *fakeSessionRepo) GetByExternalAddrPort(addr netip.AddrPort) (*session.Peer, error) {
	s, ok := r.sessions[addr]
	if !ok {
		return nil, errors.New("no session")
	}
	return s, nil
}
func (r *fakeSessionRepo) FindByDestinationIP(_ netip.Addr) (*session.Peer, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.returnPeer, nil
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

// testSession is a lightweight mock implementing connection.Session for tests.
type testSession struct {
	crypto     connection.Crypto
	internalIP netip.Addr
	externalIP netip.AddrPort
}

func (s *testSession) Crypto() connection.Crypto        { return s.crypto }
func (s *testSession) InternalAddr() netip.Addr         { return s.internalIP }
func (s *testSession) ExternalAddrPort() netip.AddrPort { return s.externalIP }
func (s *testSession) RekeyController() rekey.FSM       { return nil }
func (s *testSession) IsSourceAllowed(netip.Addr) bool  { return true }

// makeValidIPv4Packet creates a minimal valid IPv4 packet with the given source IP.
// Used in tests to satisfy AllowedIPs validation.
func makeValidIPv4Packet(srcIP netip.Addr) []byte {
	packet := make([]byte, 20) // Minimum IPv4 header
	packet[0] = 0x45           // Version 4, IHL 5 (20 bytes)
	src := srcIP.As4()
	copy(packet[12:16], src[:])
	return packet
}

/* ========= tests ========= */

func TestHandleTransport_CtxDoneBeforeAccept_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener := &fakeTcpListenerCtxDone{ctx: ctx, t: t}
	logger := &fakeLogger{}
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: 7777},
		&fakeWriter{},
		listener,
		repo,
		logger,
		registrar,
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
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: 7777},
		&fakeWriter{},
		listener,
		repo,
		logger,
		registrar,
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
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: 1111},
		&fakeWriter{},
		listener,
		repo,
		logger,
		registrar,
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
	h := &fakeHandshake{clientID: 1, err: errors.New("handshake fail")}
	handshakeFactory := &fakeHandshakeFactory{hs: h}
	repo := &fakeSessionRepo{}

	registrar := tcp_registration.NewRegistrar(logger, handshakeFactory, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(ctx, settings.Settings{Port: 2222}, &fakeWriter{}, listener, repo, logger, registrar)
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
	addr := tcpAddr("127.0.0.1", 9991)
	fconn := &fakeConn{addr: addr}
	logger := &fakeLogger{}
	hs := &fakeHandshake{clientID: 1, err: errors.New("boom")}
	hf := &fakeHandshakeFactory{hs: hs}

	registrar := tcp_registration.NewRegistrar(logger, hf, &fakeCryptoFactory{}, &fakeSessionRepo{}, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	_, _, err := registrar.RegisterClient(fconn)
	if err == nil || !strings.Contains(err.Error(), "client 127.0.0.1:9991 failed registration: boom") {
		t.Fatalf("expected handshake error, got %v", err)
	}
}

func TestRegisterClient_CryptoFactoryError(t *testing.T) {
	addr := tcpAddr("127.0.0.1", 9999)
	fconn := &fakeConn{addr: addr}
	logger := &fakeLogger{}
	h := &fakeHandshake{clientID: 1}
	handshakeFactory := &fakeHandshakeFactory{hs: h}

	registrar := tcp_registration.NewRegistrar(logger, handshakeFactory, &fakeCryptoFactory{err: errors.New("crypto fail")}, &fakeSessionRepo{}, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	_, _, err := registrar.RegisterClient(fconn)
	if err == nil || err.Error() != "client 127.0.0.1:9999 failed registration: crypto fail" {
		t.Errorf("expected crypto error, got %v", err)
	}
}

type badAddrConn struct{ fakeConn }

func (c *badAddrConn) RemoteAddr() net.Addr { return &struct{ net.Addr }{} }

func TestRegisterClient_BadAddrType(t *testing.T) {
	fconn := &badAddrConn{fakeConn{addr: nil}}
	logger := &fakeLogger{}
	h := &fakeHandshake{clientID: 1}
	handshakeFactory := &fakeHandshakeFactory{hs: h}

	registrar := tcp_registration.NewRegistrar(logger, handshakeFactory, &fakeCryptoFactory{}, &fakeSessionRepo{}, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	_, _, err := registrar.RegisterClient(fconn)
	if err == nil || !strings.HasPrefix(err.Error(), "invalid remote address type") {
		t.Errorf("expected 'invalid remote address type' error, got '%v'", err)
	}
}

func TestRegisterClient_NegativeClientID_FailsAllocation(t *testing.T) {
	// Negative clientID causes AllocateClientIP to fail.
	addr := tcpAddr("127.0.0.1", 8080)
	fconn := &fakeConn{addr: addr}
	logger := &fakeLogger{}
	h := &fakeHandshake{clientID: -1} // invalid
	handshakeFactory := &fakeHandshakeFactory{hs: h}

	registrar := tcp_registration.NewRegistrar(logger, handshakeFactory, &fakeCryptoFactory{}, &fakeSessionRepo{getErr: session.ErrNotFound}, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	peer, transport, err := registrar.RegisterClient(fconn)
	if err == nil {
		t.Fatal("expected error from IP allocation with zero clientID")
	}
	if peer != nil || transport != nil {
		t.Fatal("expected nil peer and transport on allocation error")
	}
}

func TestRegisterClient_SessionRepoGetError(t *testing.T) {
	addr := tcpAddr("127.0.0.1", 9090)
	fconn := &fakeConn{addr: addr}
	logger := &fakeLogger{}
	h := &fakeHandshake{clientID: 1}
	handshakeFactory := &fakeHandshakeFactory{hs: h}
	repo := &fakeSessionRepo{getErr: errors.New("db down")}

	registrar := tcp_registration.NewRegistrar(logger, handshakeFactory, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	_, _, err := registrar.RegisterClient(fconn)
	if err == nil || !strings.HasPrefix(err.Error(), "connection closed") {
		t.Errorf("expected 'connection closed', got '%v'", err)
	}
}

func TestRegisterClient_ReplaceSession(t *testing.T) {
	addr := tcpAddr("127.0.0.1", 8070)
	fconn := &fakeConn{addr: addr}
	logger := &fakeLogger{}
	h := &fakeHandshake{clientID: 1}
	handshakeFactory := &fakeHandshakeFactory{hs: h}

	oldSess := &testSession{externalIP: mustAddrPort("127.0.0.1:8070")}
	oldPeer := session.NewPeer(oldSess, &noopEgress{})
	srepo := &fakeSessionRepo{
		getErr:     nil,
		returnPeer: oldPeer, // existing session -> will be deleted
	}

	registrar := tcp_registration.NewRegistrar(logger, handshakeFactory, &fakeCryptoFactory{}, srepo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	_, _, _ = registrar.RegisterClient(fconn)

	// We expect exactly one Delete call: old replaced session.
	// (The new session's cleanup is now handled by the caller via handleClient.)
	if len(srepo.deleted) != 1 {
		t.Fatalf("expected 1 Delete call (old session), got %d: %+v", len(srepo.deleted), srepo.deleted)
	}
	del := srepo.deleted[0]
	if del.ExternalAddrPort().Addr().Unmap() != oldPeer.ExternalAddrPort().Addr().Unmap() ||
		del.ExternalAddrPort().Port() != oldPeer.ExternalAddrPort().Port() {
		t.Errorf("deleted session has wrong externalIP: %v, want %v", del.ExternalAddrPort(), oldPeer.ExternalAddrPort())
	}
}

func TestRegisterClient_AddsSessionOnNotFound(t *testing.T) {
	addr := tcpAddr("127.0.0.1", 6010)
	fconn := &fakeConn{addr: addr}
	logger := &fakeLogger{}
	h := &fakeHandshake{clientID: 1}
	handshakeFactory := &fakeHandshakeFactory{hs: h}
	repo := &fakeSessionRepo{getErr: session.ErrNotFound}

	registrar := tcp_registration.NewRegistrar(logger, handshakeFactory, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	if _, _, err := registrar.RegisterClient(fconn); err != nil {
		t.Fatalf("RegisterClient returned error: %v", err)
	}
	if len(repo.added) != 1 {
		t.Fatalf("expected 1 added session, got %d", len(repo.added))
	}
	added := repo.added[0]
	if added.ExternalAddrPort().Addr().Unmap().String() != "127.0.0.1" {
		t.Errorf("added session addr mismatch: %v", added.ExternalAddrPort())
	}
}

func TestHandleClient_RekeyInit_DispatchedToControlPlane(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build a valid RekeyInit packet.
	crypto := &primitives.DefaultKeyDeriver{}
	pub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	rekeyPkt := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, rekeyPkt)
	copy(rekeyPkt[3:], pub)

	rk := &tcpTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), true)

	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5559),
		readBufs: [][]byte{rekeyPkt},
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := &testSession{
		crypto:     &fakeCrypto{},
		internalIP: netip.MustParseAddr("10.0.0.9"),
		externalIP: mustAddrPort("1.2.3.4:5559"),
	}
	eg := &noopEgress{}
	peer := session.NewPeer(sess, eg)
	// Set rekey controller on the real Session so RekeyController() returns non-nil.
	realSess := session.NewSession(&fakeCrypto{}, fsm, netip.MustParseAddr("10.0.0.9"), mustAddrPort("1.2.3.4:5559"))
	peer = session.NewPeer(realSess, eg)

	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	h := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)
	h.(*TransportHandler).handleClient(ctx, peer, conn, writer)

	// The RekeyInit should have been dispatched to controlplane.
	// Verify no TUN write happened (controlplane packets are consumed).
	if len(writer.wrote) != 0 {
		t.Fatalf("expected no TUN writes for RekeyInit (should be consumed by controlplane), got %d", len(writer.wrote))
	}
	if fsm.LastRekeyEpoch == 0 {
		t.Fatalf("expected rekey to be processed and epoch activated, got LastRekeyEpoch=%d", fsm.LastRekeyEpoch)
	}
}

func TestHandleClient_CtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &fakeConn{addr: tcpAddr("1.2.3.4", 5555)}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := &testSession{crypto: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.5"), externalIP: mustAddrPort("1.2.3.4:5555")}
	peer := session.NewPeer(sess, nil)
	repo := &fakeSessionRepo{}

	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)
	handler.(*TransportHandler).handleClient(ctx, peer, conn, writer)

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
	sess := &testSession{crypto: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.6"), externalIP: mustAddrPort("1.2.3.4:5556")}
	peer := session.NewPeer(sess, nil)
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)
	handler.(*TransportHandler).handleClient(ctx, peer, conn, writer)

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
	sess := &testSession{crypto: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.7"), externalIP: mustAddrPort("1.2.3.4:5557")}
	peer := session.NewPeer(sess, nil)
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)
	handler.(*TransportHandler).handleClient(ctx, peer, conn, writer)

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
	sess := &testSession{crypto: &fakeCrypto{}, internalIP: netip.MustParseAddr("10.0.0.8"), externalIP: mustAddrPort("1.2.3.4:5558")}
	peer := session.NewPeer(sess, nil)
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)
	handler.(*TransportHandler).handleClient(ctx, peer, conn, writer)

	if !logger.contains("invalid ciphertext length:") {
		t.Errorf("expected invalid length log, got %v", logger.logs)
	}
}

func TestHandleClient_DecryptError_ClosesConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5560),
		readBufs: [][]byte{make([]byte, chacha20poly1305.Overhead)}, // valid length
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := &testSession{crypto: &fakeCrypto{decErr: errors.New("bad decrypt")}, internalIP: netip.MustParseAddr("10.0.0.10"), externalIP: mustAddrPort("1.2.3.4:5560")}
	peer := session.NewPeer(sess, nil)
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)
	handler.(*TransportHandler).handleClient(ctx, peer, conn, writer)

	if !logger.contains("failed to decrypt data: bad decrypt") {
		t.Errorf("expected decrypt error log, got %v", logger.logs)
	}
	if len(repo.deleted) != 1 {
		t.Fatalf("expected session deleted on decrypt error, got %d deletes", len(repo.deleted))
	}
	if !conn.closed {
		t.Fatal("expected transport Close() on decrypt error")
	}
}

func TestHandleClient_WriteTunError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	internalIP := netip.MustParseAddr("10.0.0.11")
	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 5561),
		readBufs: [][]byte{makeValidIPv4Packet(internalIP)}, // valid IPv4 packet
	}
	writer := &fakeWriter{err: errors.New("fail tun")}
	logger := &fakeLogger{}
	sess := &testSession{crypto: &fakeCrypto{}, internalIP: internalIP, externalIP: mustAddrPort("1.2.3.4:5561")}
	peer := session.NewPeer(sess, nil)
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	handler := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)
	handler.(*TransportHandler).handleClient(ctx, peer, conn, writer)

	if !logger.contains("failed to write to TUN: fail tun") {
		t.Errorf("expected write-to-tun error log, got %v", logger.logs)
	}
}

func TestHandleClient_HappyDataPath_WritesToTun_AndCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	internalIP := netip.MustParseAddr("10.0.0.12")
	payload := makeValidIPv4Packet(internalIP) // valid IPv4 packet

	conn := &fakeConn{
		addr:     tcpAddr("1.2.3.4", 6001),
		readBufs: [][]byte{payload},
	}
	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sess := &testSession{
		crypto:     &fakeCrypto{},
		internalIP: internalIP,
		externalIP: mustAddrPort("1.2.3.4:6001"),
	}
	peer := session.NewPeer(sess, nil)
	repo := &fakeSessionRepo{}
	registrar := tcp_registration.NewRegistrar(logger, &fakeHandshakeFactory{}, &fakeCryptoFactory{}, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	h := NewTransportHandler(context.Background(), settings.Settings{}, writer, &fakeTcpListener{}, repo, logger, registrar)

	h.(*TransportHandler).handleClient(ctx, peer, conn, writer)

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
