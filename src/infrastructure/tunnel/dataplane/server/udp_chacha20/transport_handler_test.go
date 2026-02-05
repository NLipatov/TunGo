package udp_chacha20

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"net/netip"
	"sync"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/network/udp/queue/udp"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/udp_registration"
)

/* ===================== Test doubles (prefixed with TransportHandler...) ===================== */

// TransportHandlerBlockingHandshake waits for transport.Read to return (typically
// when the registration queue is closed) and exposes the resulting error.
type TransportHandlerBlockingHandshake struct {
	errCh chan error
}

func (b *TransportHandlerBlockingHandshake) Id() [32]byte              { return [32]byte{} }
func (b *TransportHandlerBlockingHandshake) KeyClientToServer() []byte { return nil }
func (b *TransportHandlerBlockingHandshake) KeyServerToClient() []byte { return nil }
func (b *TransportHandlerBlockingHandshake) ServerSideHandshake(t connection.Transport) (netip.Addr, error) {
	buf := make([]byte, 1)
	_, err := t.Read(buf)
	if b.errCh != nil {
		b.errCh <- err
		close(b.errCh)
	}
	return netip.Addr{}, err
}
func (b *TransportHandlerBlockingHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	return nil
}

// TransportHandlerQueueReader consumes a queue and reports when Unblocked.
type TransportHandlerQueueReader struct {
	q  *udp.RegistrationQueue
	ch chan error
}

func (r *TransportHandlerQueueReader) Run() {
	dst := make([]byte, 32)
	_, err := r.q.ReadInto(dst)
	r.ch <- err
	close(r.ch)
}

type failingCryptoFactory struct{}

func (f failingCryptoFactory) FromHandshake(_ connection.Handshake, _ bool) (connection.Crypto, *rekey.StateMachine, error) {
	return nil, nil, errors.New("crypto init fail")
}

// Slow handshake mock: simulates long-running ServerSideHandshake
type TransportHandlerSlowHandshake struct {
	delay                         time.Duration
	ip                            netip.Addr
	err                           error
	TransportHandlerFakeHandshake // embed
}

func (s *TransportHandlerSlowHandshake) ServerSideHandshake(_ connection.Transport) (netip.Addr, error) {
	time.Sleep(s.delay)
	return s.ip, s.err
}

// TransportHandlerAlwaysWriteCrypto passes data through unchanged.
type TransportHandlerAlwaysWriteCrypto struct{}

func (d *TransportHandlerAlwaysWriteCrypto) Encrypt(in []byte) ([]byte, error) { return in, nil }
func (d *TransportHandlerAlwaysWriteCrypto) Decrypt(in []byte) ([]byte, error) { return in, nil }

// TransportHandlerFakeAEAD is a trivial AEAD for chacha20 builder.
type TransportHandlerFakeAEAD struct{}

func (f TransportHandlerFakeAEAD) NonceSize() int { return 12 }
func (f TransportHandlerFakeAEAD) Overhead() int  { return 0 }
func (f TransportHandlerFakeAEAD) Seal(dst, nonce, plaintext, ad []byte) []byte {
	_ = nonce
	_ = ad
	out := make([]byte, len(dst)+len(plaintext))
	copy(out, dst)
	copy(out[len(dst):], plaintext)
	return out
}
func (f TransportHandlerFakeAEAD) Open(dst, nonce, ciphertext, ad []byte) ([]byte, error) {
	_ = nonce
	_ = ad
	out := make([]byte, len(dst)+len(ciphertext))
	copy(out, dst)
	copy(out[len(dst):], ciphertext)
	return out, nil
}

// TransportHandlerMockAEADBuilder adapts to chacha20.NewUdpSessionBuilder.
type TransportHandlerMockAEADBuilder struct{}

func (TransportHandlerMockAEADBuilder) FromHandshake(h connection.Handshake, isServer bool) (cipher.AEAD, cipher.AEAD, error) {
	_ = h
	_ = isServer
	return TransportHandlerFakeAEAD{}, TransportHandlerFakeAEAD{}, nil
}

// TransportHandlerFakeUdpListener implements listeners.UdpListener.
type TransportHandlerFakeUdpListener struct {
	readMu    sync.Mutex
	readIdx   int
	readBufs  [][]byte
	readAddrs []netip.AddrPort

	// write capture
	writes []struct {
		data []byte
		addr netip.AddrPort
	}
	writeCh chan struct{}

	closed            bool
	setReadBufferCnt  int
	setWriteBufferCnt int
}

func (f *TransportHandlerFakeUdpListener) Close() error { f.closed = true; return nil }
func (f *TransportHandlerFakeUdpListener) SetReadBuffer(_ int) error {
	f.setReadBufferCnt++
	return nil
}
func (f *TransportHandlerFakeUdpListener) SetWriteBuffer(_ int) error {
	f.setWriteBufferCnt++
	return nil
}
func (f *TransportHandlerFakeUdpListener) WriteToUDPAddrPort(data []byte, addr netip.AddrPort) (int, error) {
	f.writes = append(f.writes, struct {
		data []byte
		addr netip.AddrPort
	}{append([]byte(nil), data...), addr})
	if f.writeCh != nil {
		select {
		case f.writeCh <- struct{}{}:
		default:
		}
	}
	return len(data), nil
}
func (f *TransportHandlerFakeUdpListener) ReadMsgUDPAddrPort(b, _ []byte) (int, int, int, netip.AddrPort, error) {
	f.readMu.Lock()
	defer f.readMu.Unlock()
	if f.readIdx >= len(f.readBufs) {
		time.Sleep(10 * time.Millisecond)
		return 0, 0, 0, netip.AddrPort{}, io.EOF
	}
	data := f.readBufs[f.readIdx]
	addr := f.readAddrs[f.readIdx]
	f.readIdx++
	copy(b, data)
	return len(data), 0, 0, addr, nil
}

// TransportHandlerBlockingUdpListener blocks in ReadMsgUDPAddrPort until Close().
// This allows testing the path where ctx is canceled during a blocking read,
// making handler return ctx.Err().
type TransportHandlerBlockingUdpListener struct {
	closed bool
	ch     chan struct{}
	addr   netip.AddrPort
}

func (b *TransportHandlerBlockingUdpListener) Close() error {
	if !b.closed {
		b.closed = true
		close(b.ch)
	}
	return nil
}
func (b *TransportHandlerBlockingUdpListener) SetReadBuffer(_ int) error  { return nil }
func (b *TransportHandlerBlockingUdpListener) SetWriteBuffer(_ int) error { return nil }
func (b *TransportHandlerBlockingUdpListener) WriteToUDPAddrPort(_ []byte, _ netip.AddrPort) (int, error) {
	return 0, nil
}
func (b *TransportHandlerBlockingUdpListener) ReadMsgUDPAddrPort(_, _ []byte) (int, int, int, netip.AddrPort, error) {
	<-b.ch // unblock when Close is called
	return 0, 0, 0, b.addr, io.ErrClosedPipe
}

// TransportHandlerFakeWriter captures writes and can inject an error.
type TransportHandlerFakeWriter struct {
	buf     bytes.Buffer
	err     error
	wrote   [][]byte
	writeCh chan struct{}
}

func (f *TransportHandlerFakeWriter) Write(p []byte) (int, error) {
	if f.writeCh != nil {
		select {
		case f.writeCh <- struct{}{}:
		default:
		}
	}
	if f.err != nil {
		return 0, f.err
	}
	f.wrote = append(f.wrote, append([]byte(nil), p...))
	return f.buf.Write(p)
}

// TransportHandlerFakeLogger collects logs (format string only, args ignored for simplicity).
type TransportHandlerFakeLogger struct {
	mu   sync.Mutex
	logs []string
}

func (l *TransportHandlerFakeLogger) Printf(format string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, format)
}
func (l *TransportHandlerFakeLogger) contains(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.logs {
		if bytes.Contains([]byte(s), []byte(sub)) {
			return true
		}
	}
	return false
}
func (l *TransportHandlerFakeLogger) count(sub string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.logs {
		if bytes.Contains([]byte(s), []byte(sub)) {
			n++
		}
	}
	return n
}

// TransportHandlerFakeHandshake & factory.
type TransportHandlerFakeHandshake struct {
	ip     netip.Addr
	err    error
	id     [32]byte
	client [32]byte
	server [32]byte
}

func (f *TransportHandlerFakeHandshake) Id() [32]byte              { return f.id }
func (f *TransportHandlerFakeHandshake) KeyClientToServer() []byte { return f.client[:] }
func (f *TransportHandlerFakeHandshake) KeyServerToClient() []byte { return f.server[:] }
func (f *TransportHandlerFakeHandshake) ServerSideHandshake(_ connection.Transport) (netip.Addr, error) {
	return f.ip, f.err
}
func (f *TransportHandlerFakeHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	return nil
}

type TransportHandlerFakeHandshakeFactory struct{ hs connection.Handshake }

func (f *TransportHandlerFakeHandshakeFactory) NewHandshake() connection.Handshake { return f.hs }

// TransportHandlerSessionRepo is a minimal repo for tests.
type TransportHandlerSessionRepo struct {
	mu       sync.Mutex
	sessions map[netip.AddrPort]*session.Peer
	adds     []*session.Peer
	afterAdd func()
}

func (r *TransportHandlerSessionRepo) Add(p *session.Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = map[netip.AddrPort]*session.Peer{}
	}
	r.sessions[p.ExternalAddrPort()] = p
	r.adds = append(r.adds, p)
	if r.afterAdd != nil {
		r.afterAdd()
	}
}
func (r *TransportHandlerSessionRepo) Delete(_ *session.Peer) {}
func (r *TransportHandlerSessionRepo) GetByInternalAddrPort(_ netip.Addr) (*session.Peer, error) {
	return nil, errors.New("not implemented")
}
func (r *TransportHandlerSessionRepo) GetByExternalAddrPort(addr netip.AddrPort) (*session.Peer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[addr]
	if !ok {
		return nil, errors.New("no session")
	}
	return s, nil
}
func (r *TransportHandlerSessionRepo) FindByDestinationIP(_ netip.Addr) (*session.Peer, error) {
	return nil, errors.New("not implemented")
}

// testSession is a lightweight mock implementing connection.Session for tests.
type testSession struct {
	crypto     connection.Crypto
	internalIP netip.Addr
	externalIP netip.AddrPort
	fsm        rekey.FSM
}

func (s *testSession) Crypto() connection.Crypto          { return s.crypto }
func (s *testSession) InternalAddr() netip.Addr           { return s.internalIP }
func (s *testSession) ExternalAddrPort() netip.AddrPort   { return s.externalIP }
func (s *testSession) RekeyController() rekey.FSM         { return s.fsm }
func (s *testSession) IsSourceAllowed(netip.Addr) bool { return true }

// makeValidIPv4Packet creates a minimal valid IPv4 packet with the given source IP.
// Used in tests to satisfy AllowedIPs validation.
func makeValidIPv4Packet(srcIP netip.Addr) []byte {
	packet := make([]byte, 20) // Minimum IPv4 header
	packet[0] = 0x45           // Version 4, IHL 5 (20 bytes)
	src := srcIP.As4()
	copy(packet[12:16], src[:])
	return packet
}

// testSendReset creates a sendReset callback that encodes a SessionReset and writes to conn.
func testSendReset(conn *TransportHandlerFakeUdpListener) func(netip.AddrPort) {
	return func(addrPort netip.AddrPort) {
		buf := make([]byte, 3)
		payload, err := service_packet.EncodeLegacyHeader(service_packet.SessionReset, buf)
		if err != nil {
			return
		}
		_, _ = conn.WriteToUDPAddrPort(payload, addrPort)
	}
}

// noopSendReset is a no-op sendReset for tests that don't test session resets.
func noopSendReset(_ netip.AddrPort) {}

/* ==================================== Tests ==================================== */

// Cancel before first read: select chooses ctx.Done() branch => returns nil.
func TestHandleTransport_CancelBeforeLoop_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	hsf := &TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}}
	conn := &TransportHandlerFakeUdpListener{} // Accept/read should not matter

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	h := NewTransportHandler(ctx, settings.Settings{Port: "7777"}, writer, conn, repo, logger, registrar)

	err := h.HandleTransport()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !conn.closed {
		t.Fatalf("expected UDP listener to be closed by defer")
	}
}

// Cancel while ReadMsgUDPAddrPort is blocked: read returns error after Close; handler returns ctx.Err().
func TestHandleTransport_CancelWhileRead_ReturnsCtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	hsf := &TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}}

	bl := &TransportHandlerBlockingUdpListener{
		ch:   make(chan struct{}),
		addr: netip.MustParseAddrPort("127.0.0.1:9001"),
	}

	registrar := udp_registration.NewRegistrar(ctx, bl, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	h := NewTransportHandler(ctx, settings.Settings{Port: "9001"}, writer, bl, repo, logger, registrar)

	errCh := make(chan error, 1)
	go func() { errCh <- h.HandleTransport() }()
	// Ensure the goroutine is inside ReadMsg.
	time.Sleep(20 * time.Millisecond)

	// Cancel -> internal goroutine calls Close() -> ReadMsg unblocks with error.
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !bl.closed {
		t.Fatalf("expected UDP listener to be closed")
	}
}

// Read error when ctx not canceled: should log and continue (we then cancel).
func TestHandleTransport_ReadMsgUDPAddrPortError_LogsAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	hsf := &TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{}, // immediate EOF
		readAddrs: []netip.AddrPort{},
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	h := NewTransportHandler(ctx, settings.Settings{Port: "4444"}, writer, conn, repo, logger, registrar)

	done := make(chan struct{})
	go func() { _ = h.HandleTransport(); close(done) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	// We can't match exact string (format uses %s), but ensure at least one failure was logged
	if logger.count("failed to read from UDP") == 0 {
		t.Fatalf("expected at least one read error log, got %v", logger.logs)
	}
}

// Empty packet -> "packet dropped" path.
func TestHandleTransport_EmptyPacket_Dropped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	hsf := &TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}}

	addr := netip.MustParseAddrPort("192.168.0.2:5555")
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{}}, // n == 0
		readAddrs: []netip.AddrPort{addr},
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	h := NewTransportHandler(ctx, settings.Settings{Port: "5555"}, writer, conn, repo, logger, registrar)

	done := make(chan struct{})
	go func() { _ = h.HandleTransport(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if !logger.contains("packet dropped: empty packet") {
		t.Fatalf("expected empty-packet log, got %v", logger.logs)
	}
}

// First packet triggers registration (handshake OK) -> session added.
func TestTransportHandler_RegistrationPacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	sessionRegistered := make(chan struct{})

	repo := &TransportHandlerSessionRepo{
		afterAdd: func() { close(sessionRegistered) },
	}

	clientAddr := netip.MustParseAddrPort("192.168.1.10:5555")
	internalIP := netip.MustParseAddr("10.0.0.5")
	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}}, // initial packet
		readAddrs: []netip.AddrPort{clientAddr},
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "9999"}, writer, conn, repo, logger, registrar)

	go func() { _ = handler.HandleTransport() }()

	select {
	case <-sessionRegistered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("timeout: session was not registered")
	}
	time.Sleep(10 * time.Millisecond)
	if len(repo.adds) != 1 {
		t.Fatalf("expected 1 session registered, got %d", len(repo.adds))
	}
	// Buffers should have been set
	if conn.setReadBufferCnt == 0 || conn.setWriteBufferCnt == 0 {
		t.Fatalf("expected SetReadBuffer/SetWriteBuffer to be called, got r=%d w=%d", conn.setReadBufferCnt, conn.setWriteBufferCnt)
	}
}

// Handshake error -> SessionReset is sent (EncodeLegacy ok).
func TestTransportHandler_HandshakeError_SendsSessionReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.20:5000")
	fakeHS := &TransportHandlerFakeHandshake{ip: netip.Addr{}, err: errors.New("hs fail")}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	writeCh := make(chan struct{}, 1)
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xab, 0xcd}},
		readAddrs: []netip.AddrPort{clientAddr},
		writeCh:   writeCh,
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		testSendReset(conn),
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "1111"}, writer, conn, repo, logger, registrar)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	select {
	case <-writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout: SessionReset was not sent")
	}
	cancel()
	<-done

	if len(conn.writes) != 1 || conn.writes[0].data[0] != byte(service_packet.SessionReset) {
		t.Errorf("expected SessionReset to be sent, got %+v", conn.writes)
	}
}

// Decrypt error on existing session -> no TUN writes, error is logged.
func TestTransportHandler_DecryptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.30:4000")
	internalIP := netip.MustParseAddr("10.0.0.10")
	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	// First packet -> registration
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	sessionRegistered := make(chan struct{})
	repo.afterAdd = func() {
		// Replace stored session with one that fails decrypt, to hit that branch.
		s := repo.adds[0]
		sess := &testSession{
			internalIP: s.InternalAddr(),
			externalIP: s.ExternalAddrPort(),
			crypto:     &transportHandlerFailingCrypto{},
		}
		repo.sessions[clientAddr] = session.NewPeer(sess, nil)
		close(sessionRegistered)
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "2222"}, writer, conn, repo, logger, registrar)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	<-sessionRegistered
	cancel()
	<-done

	if len(writer.wrote) != 0 {
		t.Errorf("expected no writes to TUN if decrypt fails")
	}
}

type transportHandlerFailingCrypto struct{}

func (t *transportHandlerFailingCrypto) Encrypt(in []byte) ([]byte, error) { return in, nil }
func (t *transportHandlerFailingCrypto) Decrypt(_ []byte) ([]byte, error) {
	return nil, errors.New("dec fail")
}

// Writer error after successful decrypt.
func TestTransportHandler_WriteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeAttempted := make(chan struct{}, 1)
	writer := &TransportHandlerFakeWriter{
		err:     errors.New("write fail"),
		writeCh: writeAttempted,
	}

	logger := &TransportHandlerFakeLogger{}
	clientAddr := netip.MustParseAddrPort("192.168.1.40:6000")
	internalIP := netip.MustParseAddr("10.0.0.40")

	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	repo := &TransportHandlerSessionRepo{sessions: make(map[netip.AddrPort]*session.Peer)}
	sessionRegistered := make(chan struct{})
	// After registration -> replace crypto for decrypted path
	repo.afterAdd = func() {
		s := repo.adds[0]
		sess := &testSession{
			internalIP: s.InternalAddr(),
			externalIP: s.ExternalAddrPort(),
			crypto:     &TransportHandlerAlwaysWriteCrypto{},
		}
		repo.sessions[clientAddr] = session.NewPeer(sess, nil)
		close(sessionRegistered)
	}

	// custom UDP listener: feeds packets one by one on demand
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}}, // first: registration
		readAddrs: []netip.AddrPort{clientAddr},
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "3333"}, writer, conn, repo, logger, registrar)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	// 1) Wait for registration to complete
	select {
	case <-sessionRegistered:
	case <-time.After(time.Second):
		t.Fatal("timeout: session not registered")
	}

	// 2) Now inject second packet dynamically - must be valid IPv4 packet with matching source IP
	validPacket := makeValidIPv4Packet(internalIP)
	conn.readMu.Lock()
	conn.readBufs = append(conn.readBufs, validPacket)
	conn.readAddrs = append(conn.readAddrs, clientAddr)
	conn.readMu.Unlock()

	// 3) Now writer should be called
	select {
	case <-writeAttempted:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timeout: expected write to be attempted")
	}
	<-done

	if len(writer.wrote) != 0 {
		t.Errorf("expected no stored writes on error, but got %x", writer.wrote)
	}
}

// Happy path: existing session, decrypt OK, data written once.
func TestTransportHandler_HappyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}

	clientAddr := netip.MustParseAddrPort("192.168.1.50:5050")
	internalIP := netip.MustParseAddr("10.0.0.50")
	repo := &TransportHandlerSessionRepo{sessions: map[netip.AddrPort]*session.Peer{}}
	sess := &testSession{
		crypto:     &TransportHandlerAlwaysWriteCrypto{},
		internalIP: internalIP,
		externalIP: clientAddr,
	}
	repo.sessions[clientAddr] = session.NewPeer(sess, nil)

	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	// Must use valid IPv4 packet with matching source IP for AllowedIPs validation
	validPacket := makeValidIPv4Packet(internalIP)
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{validPacket},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "5050"}, writer, conn, repo, logger, registrar)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if len(writer.wrote) != 1 {
		t.Fatalf("expected 1 packet written to TUN, got %d", len(writer.wrote))
	}
}

// NAT rebinding: new addr -> registration is triggered and session is added.
func TestTransportHandler_NATRebinding_ReRegister(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}

	oldAddr := netip.MustParseAddrPort("192.168.1.51:5050")
	newAddr := netip.MustParseAddrPort("192.168.1.51:6060")
	internalIP := netip.MustParseAddr("10.0.0.51")

	repo := &TransportHandlerSessionRepo{sessions: map[netip.AddrPort]*session.Peer{}}
	oldSess := &testSession{
		crypto:     &TransportHandlerAlwaysWriteCrypto{},
		internalIP: internalIP,
		externalIP: oldAddr,
	}
	repo.sessions[oldAddr] = session.NewPeer(oldSess, nil)

	sessionRegistered := make(chan struct{})
	repo.afterAdd = func() { close(sessionRegistered) }

	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xca, 0xfe}}, // new addr will force re-registration
		readAddrs: []netip.AddrPort{newAddr},
	}
	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "6060"}, writer, conn, repo, logger, registrar)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	select {
	case <-sessionRegistered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("timeout: session was not re-registered")
	}
	<-done
}

// registerClient with zero IP -> session is still added (zero value is valid in netip.Addr)
func TestTransportHandler_RegisterClient_ZeroInternalIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	sessionRegistered := make(chan struct{})
	repo := &TransportHandlerSessionRepo{
		afterAdd: func() { close(sessionRegistered) },
	}
	clientAddr := netip.MustParseAddrPort("192.168.1.60:6000")

	fakeHS := &TransportHandlerFakeHandshake{ip: netip.Addr{}} // zero value
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xde, 0xad}}, // needs >= 2 bytes to pass epoch check
		readAddrs: []netip.AddrPort{clientAddr},
	}
	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "6000"}, writer, conn, repo, logger, registrar)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	select {
	case <-sessionRegistered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("timeout: session was not registered")
	}
	<-done

	if len(repo.adds) != 1 {
		t.Errorf("expected 1 session registered with zero IP, got %d", len(repo.adds))
	}
}

// Sanity check for fake AEAD roundtrip to avoid dead code in tests.
func TestTransportHandlerFakeAEAD_Roundtrip(t *testing.T) {
	var f TransportHandlerFakeAEAD
	nonce := make([]byte, f.NonceSize())
	_, _ = rand.Read(nonce)
	pt := []byte("ping")
	ct := f.Seal(nil, nonce, pt, nil)
	got, err := f.Open(nil, nonce, ct, nil)
	if err != nil {
		t.Fatalf("Open err: %v", err)
	}
	if !bytes.Equal(got, pt) {
		t.Fatalf("roundtrip mismatch: %q vs %q", got, pt)
	}
}

func TestTransportHandler_getOrCreateRegistrationQueue_ExistingQueue(t *testing.T) {
	ctx := context.Background()
	r := udp_registration.NewRegistrar(ctx, &TransportHandlerFakeUdpListener{},
		&TransportHandlerSessionRepo{}, &TransportHandlerFakeLogger{},
		&TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}},
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)

	addr := netip.MustParseAddrPort("1.2.3.4:9999")

	original, isNew := r.GetOrCreateRegistrationQueue(addr)
	if !isNew {
		t.Fatalf("expected new queue on first call")
	}

	q, isNew2 := r.GetOrCreateRegistrationQueue(addr)
	if isNew2 {
		t.Fatalf("expected existing queue on second call, got new=true")
	}
	if q != original {
		t.Fatalf("expected same queue instance")
	}
}

func TestTransportHandler_SecondPacketGoesToExistingRegistrationQueue_NoNewGoroutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}

	clientAddr := netip.MustParseAddrPort("192.168.1.70:7000")

	// Two packets from same client before handshake finishes
	conn := &TransportHandlerFakeUdpListener{
		readBufs: [][]byte{
			{0x00, 0xaa}, // include epoch byte to pass length check
			{0x00, 0xbb},
		},
		readAddrs: []netip.AddrPort{clientAddr, clientAddr},
	}

	fakeHS := &TransportHandlerSlowHandshake{
		delay: 100 * time.Millisecond, // handshake runs slow, queue remains alive
		ip:    netip.MustParseAddr("10.0.0.70"),
	}

	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		handshakeFactory, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "7000"}, writer, conn, repo, logger, registrar)

	go func() {
		_ = handler.HandleTransport()
	}()

	// allow both packets to be queued before handshake finishes
	time.Sleep(50 * time.Millisecond)

	impl := handler.(*TransportHandler)
	regs := impl.registrar.Registrations()
	q := regs[clientAddr]

	if q == nil {
		t.Fatalf("expected registration queue to exist (handshake has not finished yet)")
	}

	dst := make([]byte, 8)

	n1, err1 := q.ReadInto(dst)
	if err1 != nil || n1 != 2 || dst[:n1][0] != 0x00 || dst[:n1][1] != 0xaa {
		t.Fatalf("first packet mismatch: %x err=%v", dst[:n1], err1)
	}

	n2, err2 := q.ReadInto(dst)
	if err2 != nil || n2 != 2 || dst[:n2][0] != 0x00 || dst[:n2][1] != 0xbb {
		t.Fatalf("second packet mismatch: %x err=%v", dst[:n2], err2)
	}
}

func TestHandleTransport_IgnoreHandlePacketError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{
		err: errors.New("write fail"),
	}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}

	clientAddr := netip.MustParseAddrPort("1.2.3.4:9999")

	// Existing session → decrypt OK → writer.Write returns error
	sess := &testSession{
		crypto:     &TransportHandlerAlwaysWriteCrypto{},
		internalIP: netip.MustParseAddr("10.0.0.1"),
		externalIP: clientAddr,
	}
	repo.sessions = map[netip.AddrPort]*session.Peer{
		clientAddr: session.NewPeer(sess, nil),
	}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0x01, 0x02}},
		readAddrs: []netip.AddrPort{clientAddr},
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		&TransportHandlerFakeHandshakeFactory{},
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "9999"}, writer, conn, repo, logger, registrar)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	// no crash → test passed
}

func TestRemoveRegistrationQueue_RemovesAndCloses(t *testing.T) {
	ctx := context.Background()
	hs := &TransportHandlerFakeHandshake{err: errors.New("fail")}
	r := udp_registration.NewRegistrar(ctx, &TransportHandlerFakeUdpListener{},
		&TransportHandlerSessionRepo{}, &TransportHandlerFakeLogger{},
		&TransportHandlerFakeHandshakeFactory{hs: hs},
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)

	addr := netip.MustParseAddrPort("1.2.3.4:9999")
	q, _ := r.GetOrCreateRegistrationQueue(addr)

	// RegisterClient calls removeRegistrationQueue in defer
	r.RegisterClient(addr, q)

	regs := r.Registrations()
	if _, ok := regs[addr]; ok {
		t.Fatal("expected queue removed")
	}

	dst := make([]byte, 10)
	_, err := q.ReadInto(dst)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after Close, got %v", err)
	}
}

func TestCloseAllRegistrations(t *testing.T) {
	ctx := context.Background()
	r := udp_registration.NewRegistrar(ctx, &TransportHandlerFakeUdpListener{},
		&TransportHandlerSessionRepo{}, &TransportHandlerFakeLogger{},
		&TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}},
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)

	a1 := netip.MustParseAddrPort("1.1.1.1:1000")
	a2 := netip.MustParseAddrPort("2.2.2.2:2000")

	q1, _ := r.GetOrCreateRegistrationQueue(a1)
	q2, _ := r.GetOrCreateRegistrationQueue(a2)

	r.CloseAll()

	for _, q := range []*udp.RegistrationQueue{q1, q2} {
		dst := make([]byte, 10)
		_, err := q.ReadInto(dst)
		if !errors.Is(err, io.EOF) {
			t.Fatalf("queue not closed: %v", err)
		}
	}

	regs := r.Registrations()
	if len(regs) != 0 {
		t.Fatalf("expected empty registrations map")
	}
}

func TestGetOrCreateRegistrationQueue_NewQueue(t *testing.T) {
	ctx := context.Background()
	r := udp_registration.NewRegistrar(ctx, &TransportHandlerFakeUdpListener{},
		&TransportHandlerSessionRepo{}, &TransportHandlerFakeLogger{},
		&TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}},
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)

	addr := netip.MustParseAddrPort("8.8.8.8:53")

	q, isNew := r.GetOrCreateRegistrationQueue(addr)
	if !isNew {
		t.Fatal("expected new queue")
	}
	if q == nil {
		t.Fatal("nil queue returned")
	}

	regs := r.Registrations()
	if regs[addr] != q {
		t.Fatal("queue not stored")
	}
}

func TestRegisterClient_CryptoError_SendsReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	client := netip.MustParseAddrPort("5.5.5.5:5555")

	hs := &TransportHandlerFakeHandshake{ip: netip.MustParseAddr("10.0.0.5")}
	hsf := &TransportHandlerFakeHandshakeFactory{hs: hs}

	writeCh := make(chan struct{}, 1)
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0x00, 0x01}}, // include epoch byte
		readAddrs: []netip.AddrPort{client},
		writeCh:   writeCh,
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		hsf, failingCryptoFactory{}, testSendReset(conn),
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "5555"}, writer, conn, repo, logger, registrar)

	go func() {
		_ = handler.HandleTransport()
	}()

	select {
	case <-writeCh:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("SessionReset not sent on crypto error")
	}
}
func TestTransportHandler_RegistrationQueueOverflow(t *testing.T) {
	q := udp.NewRegistrationQueue(1)

	// capacity=1 → first enqueue OK, second dropped
	q.Enqueue([]byte{0x01})
	q.Enqueue([]byte{0x02}) // overflow

	dst := make([]byte, 5)

	// First ReadInto returns first packet
	n, err := q.ReadInto(dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 || dst[0] != 0x01 {
		t.Fatalf("expected only first packet, got %x", dst[:n])
	}

	// Now queue is empty → next ReadInto would block.
	// So we must close it.
	q.Close()

	// After Close → ReadInto returns EOF immediately.
	_, err = q.ReadInto(dst)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after Close, got %v", err)
	}
}

func TestTransportHandler_RegistrationQueue_CloseUnblocksRead(t *testing.T) {
	q := udp.NewRegistrationQueue(2)

	reader := &TransportHandlerQueueReader{
		q:  q,
		ch: make(chan error, 1),
	}
	go reader.Run()

	time.Sleep(20 * time.Millisecond)
	q.Close()

	err := <-reader.ch
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF from unblocked ReadInto, got %v", err)
	}
}

// RegisterClient should terminate when the handler context is canceled and the
// registration queue is closed.
func TestRegisterClient_CanceledContextClosesQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := netip.MustParseAddrPort("1.2.3.4:1234")
	queue := udp.NewRegistrationQueue(1)

	errCh := make(chan error, 1)
	hs := &TransportHandlerBlockingHandshake{errCh: errCh}

	registrar := udp_registration.NewRegistrar(
		ctx,
		&TransportHandlerFakeUdpListener{},
		&TransportHandlerSessionRepo{},
		&TransportHandlerFakeLogger{},
		&TransportHandlerFakeHandshakeFactory{hs: hs},
		failingCryptoFactory{},
		noopSendReset,
	)

	// Pre-populate the queue in the registrar's internal map
	regs := registrar.Registrations()
	regs[addr] = queue

	done := make(chan struct{})
	go func() {
		registrar.RegisterClient(addr, queue)
		close(done)
	}()

	// Allow registerClient to start and block on handshake read.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("registerClient did not exit after context cancel")
	}

	if _, err := queue.ReadInto(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("expected closed queue after cancel, got %v", err)
	}

	regs = registrar.Registrations()
	if _, ok := regs[addr]; ok {
		t.Fatalf("registration entry for %v was not removed", addr)
	}

	if err := <-errCh; !errors.Is(err, io.EOF) {
		t.Fatalf("handshake did not observe queue closure: %v", err)
	}
}

func TestHandleTransport_ShortPacket_Logged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	hsf := &TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}}

	clientAddr := netip.MustParseAddrPort("192.168.1.99:9999")
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0x01}}, // 1-byte packet: too short for epoch
		readAddrs: []netip.AddrPort{clientAddr},
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "9999"}, writer, conn, repo, logger, registrar)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if !logger.contains("packet too short for epoch") {
		t.Fatalf("expected short packet log, got %v", logger.logs)
	}
}

func TestHandleTransport_EpochExhausted_SendsSessionReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}

	clientAddr := netip.MustParseAddrPort("192.168.1.80:8080")
	internalIP := netip.MustParseAddr("10.0.0.80")

	rk := &rekey.StateMachine{}
	rk.LastRekeyEpoch = 65001

	sess := &testSession{
		crypto:     &TransportHandlerAlwaysWriteCrypto{},
		internalIP: internalIP,
		externalIP: clientAddr,
		fsm:        rk,
	}
	repo := &TransportHandlerSessionRepo{sessions: map[netip.AddrPort]*session.Peer{
		clientAddr: session.NewPeer(sess, nil),
	}}

	hsf := &TransportHandlerFakeHandshakeFactory{hs: &TransportHandlerFakeHandshake{}}

	// Build a RekeyInit service packet that, once decrypted, triggers EpochExhausted.
	rekeyPayload := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, rekeyPayload)

	writeCh := make(chan struct{}, 1)
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{rekeyPayload},
		readAddrs: []netip.AddrPort{clientAddr},
		writeCh:   writeCh,
	}

	registrar := udp_registration.NewRegistrar(ctx, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}), noopSendReset,
	)
	handler := NewTransportHandler(ctx, settings.Settings{Port: "8080"}, writer, conn, repo, logger, registrar)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	// Wait for either the write (session reset) or timeout.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Check that handlePacket error was logged.
	if !logger.contains("failed to handle packet") {
		t.Logf("logs: %v", logger.logs)
	}
}

func TestHandleTransport_NilRegistrar_NoUnknownPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.90:9090")

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xde, 0xad}},
		readAddrs: []netip.AddrPort{clientAddr},
	}

	// nil registrar — unknown client should not panic.
	handler := NewTransportHandler(ctx, settings.Settings{Port: "9090"}, writer, conn, repo, logger, nil)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	// No panic = success.
}

func TestSendSessionReset_WritesToUDP(t *testing.T) {
	logger := &TransportHandlerFakeLogger{}
	conn := &TransportHandlerFakeUdpListener{}
	h := &TransportHandler{
		logger:       logger,
		listenerConn: conn,
	}
	addr := netip.MustParseAddrPort("192.168.1.1:12345")
	h.sendSessionReset(addr)

	if len(conn.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(conn.writes))
	}
	w := conn.writes[0]
	if w.addr != addr {
		t.Fatalf("expected write to %v, got %v", addr, w.addr)
	}
	if len(w.data) < 1 || w.data[0] != byte(service_packet.SessionReset) {
		t.Fatalf("expected SessionReset byte, got %v", w.data)
	}
}
