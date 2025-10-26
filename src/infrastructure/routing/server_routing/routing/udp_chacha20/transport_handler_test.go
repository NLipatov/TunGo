package udp_chacha20

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"tungo/application/network/connection"
	"tungo/domain/network/service"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/settings"
)

/* ===================== Test doubles (prefixed with TransportHandler...) ===================== */

// TransportHandlerServicePacketEncodeErrMock forces EncodeLegacy to fail.
type TransportHandlerServicePacketEncodeErrMock struct {
	called chan struct{}
}

func (s *TransportHandlerServicePacketEncodeErrMock) TryParseType(_ []byte) (service.PacketType, bool) {
	return service.Unknown, false
}
func (s *TransportHandlerServicePacketEncodeErrMock) EncodeLegacy(_ service.PacketType, _ []byte) ([]byte, error) {
	select {
	case s.called <- struct{}{}:
	default:
	}
	return nil, errors.New("encode failed")
}
func (s *TransportHandlerServicePacketEncodeErrMock) EncodeV1(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}

// TransportHandlerServicePacketMock encodes SessionReset as first byte = 1.
type TransportHandlerServicePacketMock struct{}

func (s *TransportHandlerServicePacketMock) TryParseType(_ []byte) (service.PacketType, bool) {
	return service.Unknown, false
}
func (s *TransportHandlerServicePacketMock) EncodeLegacy(_ service.PacketType, buffer []byte) ([]byte, error) {
	buffer[0] = byte(service.SessionReset)
	return buffer, nil
}
func (s *TransportHandlerServicePacketMock) EncodeV1(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
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
func (b *TransportHandlerBlockingUdpListener) ReadMsgUDPAddrPort(bbuf, _ []byte) (int, int, int, netip.AddrPort, error) {
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
	ip     net.IP
	err    error
	id     [32]byte
	client [32]byte
	server [32]byte
	mtu    int
	hasMTU bool
}

func (f *TransportHandlerFakeHandshake) Id() [32]byte              { return f.id }
func (f *TransportHandlerFakeHandshake) KeyClientToServer() []byte { return f.client[:] }
func (f *TransportHandlerFakeHandshake) KeyServerToClient() []byte { return f.server[:] }
func (f *TransportHandlerFakeHandshake) ServerSideHandshake(_ connection.Transport) (net.IP, error) {
	return f.ip, f.err
}
func (f *TransportHandlerFakeHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	return nil
}
func (f *TransportHandlerFakeHandshake) PeerMTU() (int, bool) {
	if !f.hasMTU {
		return 0, false
	}
	return f.mtu, true
}

type TransportHandlerFakeHandshakeFactory struct{ hs connection.Handshake }

func (f *TransportHandlerFakeHandshakeFactory) NewHandshake() connection.Handshake { return f.hs }

// TransportHandlerSessionRepo is a minimal repo for tests.
type TransportHandlerSessionRepo struct {
	mu       sync.Mutex
	sessions map[netip.AddrPort]connection.Session
	adds     []connection.Session
	deletes  []connection.Session
	afterAdd func()
}

func (r *TransportHandlerSessionRepo) Add(s connection.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = map[netip.AddrPort]connection.Session{}
	}
	r.sessions[s.ExternalAddrPort()] = s
	r.adds = append(r.adds, s)
	if r.afterAdd != nil {
		r.afterAdd()
	}
}
func (r *TransportHandlerSessionRepo) Delete(s connection.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletes = append(r.deletes, s)
	if r.sessions != nil {
		delete(r.sessions, s.ExternalAddrPort())
	}
}
func (r *TransportHandlerSessionRepo) GetByInternalAddrPort(_ netip.Addr) (connection.Session, error) {
	return Session{}, errors.New("not implemented")
}
func (r *TransportHandlerSessionRepo) GetByExternalAddrPort(addr netip.AddrPort) (connection.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[addr]
	if !ok {
		return nil, errors.New("no session")
	}
	return s, nil
}

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

	h := NewTransportHandler(ctx, settings.Settings{Port: "7777"}, writer, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)

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

	h := NewTransportHandler(ctx, settings.Settings{Port: "9001"}, writer, bl, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)

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

	h := NewTransportHandler(ctx, settings.Settings{Port: "4444"}, writer, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)

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

	h := NewTransportHandler(ctx, settings.Settings{Port: "5555"}, writer, conn, repo, logger,
		hsf, chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)

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
	internalIP := net.ParseIP("10.0.0.5")
	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}}, // initial packet
		readAddrs: []netip.AddrPort{clientAddr},
	}

	serverSettings := settings.Settings{Port: "9999", MTU: settings.DefaultEthernetMTU}
	handler := NewTransportHandler(
		ctx,
		serverSettings,
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)

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
	if mtu := repo.adds[0].MTU(); mtu != serverSettings.MTU {
		t.Fatalf("expected session MTU %d, got %d", serverSettings.MTU, mtu)
	}
	// Buffers should have been set
	if conn.setReadBufferCnt == 0 || conn.setWriteBufferCnt == 0 {
		t.Fatalf("expected SetReadBuffer/SetWriteBuffer to be called, got r=%d w=%d", conn.setReadBufferCnt, conn.setWriteBufferCnt)
	}
}

func TestTransportHandler_RegistrationPacket_UsesPeerMTU(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	sessionRegistered := make(chan struct{})

	repo := &TransportHandlerSessionRepo{
		afterAdd: func() { close(sessionRegistered) },
	}

	clientAddr := netip.MustParseAddrPort("192.168.1.11:5556")
	internalIP := net.ParseIP("10.0.0.6")
	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP, mtu: settings.SafeMTU, hasMTU: true}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xaa, 0xbb, 0xcc, 0xdd}},
		readAddrs: []netip.AddrPort{clientAddr},
	}

	serverSettings := settings.Settings{Port: "9998", MTU: settings.DefaultEthernetMTU}
	handler := NewTransportHandler(
		ctx,
		serverSettings,
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)

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
	if mtu := repo.adds[0].MTU(); mtu != settings.SafeMTU {
		t.Fatalf("expected session MTU %d, got %d", settings.SafeMTU, mtu)
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
	fakeHS := &TransportHandlerFakeHandshake{ip: nil, err: errors.New("hs fail")}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	writeCh := make(chan struct{}, 1)
	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xab, 0xcd}},
		readAddrs: []netip.AddrPort{clientAddr},
		writeCh:   writeCh,
	}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "1111"},
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	select {
	case <-writeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout: SessionReset was not sent")
	}
	cancel()
	<-done

	if len(conn.writes) != 1 || conn.writes[0].data[0] != byte(service.SessionReset) {
		t.Errorf("expected SessionReset to be sent, got %+v", conn.writes)
	}
}

// EncodeLegacy error path -> no UDP write, but EncodeLegacy called.
func TestTransportHandler_HandshakeError_ServicePacketEncodeError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.99:5999")

	fakeHS := &TransportHandlerFakeHandshake{ip: nil, err: errors.New("hs fail")}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xab, 0xcd}},
		readAddrs: []netip.AddrPort{clientAddr},
	}

	sp := &TransportHandlerServicePacketEncodeErrMock{called: make(chan struct{}, 1)}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "5999"},
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		sp,
	)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	select {
	case <-sp.called:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timeout waiting for EncodeLegacy to be called")
	}
	<-done

	if got := len(conn.writes); got != 0 {
		t.Fatalf("expected no UDP writes on EncodeLegacy error, got %d", got)
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
	internalIP := net.ParseIP("10.0.0.10")
	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	// First packet -> registration
	conn := &TransportHandlerFakeUdpListener{
		readBufs: [][]byte{
			{0xde, 0xad, 0xbe, 0xef}, // registration packet
			{0xca, 0xfe},             // triggers decrypt failure
		},
		readAddrs: []netip.AddrPort{clientAddr, clientAddr},
	}
	sessionRegistered := make(chan struct{})
	repo.afterAdd = func() {
		// Replace stored session with one that fails decrypt, to hit that branch.
		s := repo.adds[0]
		repo.sessions[clientAddr] = Session{
			internalIP: s.InternalAddr(),
			externalIP: s.ExternalAddrPort(),
			crypto:     &transportHandlerFailingCrypto{}, // custom failing crypto
			mtu:        settings.DefaultEthernetMTU,
		}
		close(sessionRegistered)
	}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "2222"},
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	<-sessionRegistered
	cancel()
	<-done

	if len(writer.wrote) != 0 {
		t.Errorf("expected no writes to TUN if decrypt fails")
	}
	if len(repo.deletes) != 1 {
		t.Fatalf("expected session to be deleted on decrypt failure, got %d deletions", len(repo.deletes))
	}
	if len(conn.writes) != 1 {
		t.Fatalf("expected session reset to be sent, got %d writes", len(conn.writes))
	}
	if conn.writes[0].data[0] != byte(service.SessionReset) {
		t.Errorf("expected session reset packet, got %v", conn.writes[0].data)
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
	internalIP := net.ParseIP("10.0.0.40")

	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	repo := &TransportHandlerSessionRepo{sessions: make(map[netip.AddrPort]connection.Session)}
	sessionRegistered := make(chan struct{})
	// After registration, replace stored session with identity-crypto for data path.
	repo.afterAdd = func() {
		s := repo.adds[0]
		repo.sessions[clientAddr] = Session{
			internalIP: s.InternalAddr(),
			externalIP: s.ExternalAddrPort(),
			crypto:     &TransportHandlerAlwaysWriteCrypto{},
			mtu:        settings.DefaultEthernetMTU,
		}
		close(sessionRegistered)
	}

	conn := &TransportHandlerFakeUdpListener{
		readBufs: [][]byte{
			{0xde, 0xad, 0xbe, 0xef}, // registration packet
			{0xba, 0xad, 0xf0, 0x0d}, // data -> write error
		},
		readAddrs: []netip.AddrPort{clientAddr, clientAddr},
	}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "3333"},
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	select {
	case <-sessionRegistered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timeout: session was not registered")
	}

	select {
	case <-writeAttempted:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timeout: expected write to be attempted")
	}
	<-done

	if len(writer.wrote) != 0 {
		t.Errorf("expected write to fail and no data to be written, but got: %x", writer.wrote)
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
	repo := &TransportHandlerSessionRepo{sessions: map[netip.AddrPort]connection.Session{}}
	repo.sessions[clientAddr] = Session{
		crypto:     &TransportHandlerAlwaysWriteCrypto{},
		internalIP: internalIP,
		externalIP: clientAddr,
		mtu:        settings.DefaultEthernetMTU,
	}

	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP.AsSlice()}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "5050"},
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)
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

	repo := &TransportHandlerSessionRepo{sessions: map[netip.AddrPort]connection.Session{}}
	repo.sessions[oldAddr] = Session{
		crypto:     &TransportHandlerAlwaysWriteCrypto{},
		internalIP: internalIP,
		externalIP: oldAddr,
		mtu:        settings.DefaultEthernetMTU,
	}

	sessionRegistered := make(chan struct{})
	repo.afterAdd = func() { close(sessionRegistered) }

	fakeHS := &TransportHandlerFakeHandshake{ip: internalIP.AsSlice()}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0xca, 0xfe}}, // new addr will force re-registration
		readAddrs: []netip.AddrPort{newAddr},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "6060"},
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)
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

// registerClient: bad internal IP slice -> no session added.
func TestTransportHandler_RegisterClient_BadInternalIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &TransportHandlerFakeWriter{}
	logger := &TransportHandlerFakeLogger{}
	repo := &TransportHandlerSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.60:6000")

	badIP := []byte{1, 2, 3} // invalid IP slice
	fakeHS := &TransportHandlerFakeHandshake{ip: badIP}
	handshakeFactory := &TransportHandlerFakeHandshakeFactory{hs: fakeHS}

	conn := &TransportHandlerFakeUdpListener{
		readBufs:  [][]byte{{0x01}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "6000"},
		writer,
		conn,
		repo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(TransportHandlerMockAEADBuilder{}),
		&TransportHandlerServicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if len(repo.adds) != 0 {
		t.Errorf("expected no session registered due to bad internal IP, got %d", len(repo.adds))
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
