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

type servicePacketEncodeErrMock struct {
	called chan struct{}
}

func (s *servicePacketEncodeErrMock) TryParseType(_ []byte) (service.PacketType, bool) {
	return service.Unknown, false
}
func (s *servicePacketEncodeErrMock) EncodeLegacy(_ service.PacketType, _ []byte) ([]byte, error) {
	select {
	case s.called <- struct{}{}:
	default:
	}
	return nil, errors.New("encode failed")
}
func (s *servicePacketEncodeErrMock) EncodeV1(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}

type servicePacketMock struct {
}

func (s *servicePacketMock) TryParseType(_ []byte) (service.PacketType, bool) {
	return service.Unknown, false
}
func (s *servicePacketMock) EncodeLegacy(_ service.PacketType, buffer []byte) ([]byte, error) {
	buffer[0] = byte(service.SessionReset)
	return buffer, nil
}
func (s *servicePacketMock) EncodeV1(_ service.PacketType, buffer []byte) ([]byte, error) {
	return buffer, nil
}

type alwaysWriteCrypto struct{}

func (d *alwaysWriteCrypto) Encrypt(in []byte) ([]byte, error) { return in, nil }
func (d *alwaysWriteCrypto) Decrypt(in []byte) ([]byte, error) { return in, nil }

type fakeAEAD struct{}

func (f fakeAEAD) NonceSize() int { return 12 }
func (f fakeAEAD) Overhead() int  { return 0 }
func (f fakeAEAD) Seal(dst, nonce, plaintext, ad []byte) []byte {
	_ = nonce
	_ = ad
	out := make([]byte, len(dst)+len(plaintext))
	copy(out, dst)
	copy(out[len(dst):], plaintext)
	return out
}
func (f fakeAEAD) Open(dst, nonce, ciphertext, ad []byte) ([]byte, error) {
	_ = nonce
	_ = ad
	out := make([]byte, len(dst)+len(ciphertext))
	copy(out, dst)
	copy(out[len(dst):], ciphertext)
	return out, nil
}

type mockAEADBuilder struct{}

func (mockAEADBuilder) FromHandshake(h connection.Handshake, isServer bool) (cipher.AEAD, cipher.AEAD, error) {
	_ = h
	_ = isServer
	return fakeAEAD{}, fakeAEAD{}, nil
}

type fakeUdpListenerConn struct {
	readMu    sync.Mutex
	readIdx   int
	readBufs  [][]byte
	readAddrs []netip.AddrPort

	writes []struct {
		data []byte
		addr netip.AddrPort
	}
	writeCh chan struct{}

	closed            bool
	setReadBufferCnt  int
	setWriteBufferCnt int
}

func (f *fakeUdpListenerConn) Close() error               { f.closed = true; return nil }
func (f *fakeUdpListenerConn) SetReadBuffer(_ int) error  { f.setReadBufferCnt++; return nil }
func (f *fakeUdpListenerConn) SetWriteBuffer(_ int) error { f.setWriteBufferCnt++; return nil }
func (f *fakeUdpListenerConn) WriteToUDPAddrPort(data []byte, addr netip.AddrPort) (int, error) {
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
func (f *fakeUdpListenerConn) ReadMsgUDPAddrPort(b, _ []byte) (int, int, int, netip.AddrPort, error) {
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

type fakeWriter struct {
	buf     bytes.Buffer
	err     error
	wrote   [][]byte
	writeCh chan struct{}
}

func (f *fakeWriter) Write(p []byte) (int, error) {
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

type testSessionRepo struct {
	sessions map[netip.AddrPort]connection.Session
	adds     []connection.Session
	afterAdd func()
}

func (r *testSessionRepo) Add(s connection.Session) {
	if r.sessions == nil {
		r.sessions = map[netip.AddrPort]connection.Session{}
	}
	r.sessions[s.ExternalAddrPort()] = s
	r.adds = append(r.adds, s)
	if r.afterAdd != nil {
		r.afterAdd()
	}
}
func (r *testSessionRepo) Delete(_ connection.Session) {}
func (r *testSessionRepo) GetByInternalAddrPort(_ netip.Addr) (connection.Session, error) {
	return Session{}, errors.New("not implemented")
}
func (r *testSessionRepo) GetByExternalAddrPort(addr netip.AddrPort) (connection.Session, error) {
	s, ok := r.sessions[addr]
	if !ok {
		return nil, errors.New("no session")
	}
	return s, nil
}

func TestTransportHandler_RegistrationPacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRegistered := make(chan struct{})

	sessionRepo := &testSessionRepo{
		afterAdd: func() { close(sessionRegistered) },
	}

	clientAddr := netip.MustParseAddrPort("192.168.1.10:5555")
	internalIP := net.ParseIP("10.0.0.5")
	fakeHS := &fakeHandshake{ip: internalIP}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}}, // test data
		readAddrs: []netip.AddrPort{clientAddr},
	}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "9999"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)

	go func() { _ = handler.HandleTransport() }()

	select {
	case <-sessionRegistered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Timeout: session was not registered")
	}
	time.Sleep(10 * time.Millisecond)
	if len(sessionRepo.adds) != 1 {
		t.Fatalf("expected 1 session registered, got %d", len(sessionRepo.adds))
	}
}

func TestTransportHandler_HandshakeError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRepo := &testSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.20:5000")
	fakeHS := &fakeHandshake{
		ip:  nil,
		err: errors.New("hs fail"),
	}
	writeCh := make(chan struct{}, 1)
	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xab, 0xcd}},
		readAddrs: []netip.AddrPort{clientAddr},
		writeCh:   writeCh,
	}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "1111"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	select {
	case <-writeCh:
	case <-time.After(time.Second):
		t.Fatal("Timeout: SessionReset was not sent")
	}
	cancel()
	<-done
	if len(conn.writes) != 1 || conn.writes[0].data[0] != 1 {
		t.Errorf("expected SessionReset to be sent, got %+v", conn.writes)
	}
}

func TestTransportHandler_DecryptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRepo := &testSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.30:4000")
	internalIP := net.ParseIP("10.0.0.10")
	fakeHS := &fakeHandshake{ip: internalIP}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	sessionRegistered := make(chan struct{})
	sessionRepo.afterAdd = func() {
		s := sessionRepo.adds[0]
		sessionRepo.sessions[clientAddr] = s
		close(sessionRegistered)
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "2222"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	<-sessionRegistered
	cancel()
	<-done

	if len(writer.wrote) != 0 {
		t.Errorf("expected no writes to TUN if decrypt fails")
	}
}

func TestTransportHandler_ReadMsgUDPAddrPortError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRepo := &testSessionRepo{}
	handshakeFactory := &fakeHandshakeFactory{hs: &fakeHandshake{}}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{},
		readAddrs: []netip.AddrPort{},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "4444"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done
}

func TestTransportHandler_WriteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeAttempted := make(chan struct{}, 1)
	writer := &fakeWriter{
		err:     errors.New("write fail"),
		writeCh: writeAttempted,
	}

	logger := &fakeLogger{}
	clientAddr := netip.MustParseAddrPort("192.168.1.40:6000")
	internalIP := net.ParseIP("10.0.0.40")

	fakeHS := &fakeHandshake{ip: internalIP}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	sessionRepo := &testSessionRepo{
		sessions: make(map[netip.AddrPort]connection.Session),
	}
	sessionRegistered := make(chan struct{})

	sessionRepo.afterAdd = func() {
		s := sessionRepo.adds[0]
		sessionRepo.sessions[clientAddr] = Session{
			internalIP: s.InternalAddr(),
			externalIP: s.ExternalAddrPort(),
			crypto:     &alwaysWriteCrypto{},
		}
		close(sessionRegistered)
	}

	conn := &fakeUdpListenerConn{
		readBufs: [][]byte{
			{0xde, 0xad, 0xbe, 0xef},
			{0xba, 0xad, 0xf0, 0x0d},
		},
		readAddrs: []netip.AddrPort{
			clientAddr,
			clientAddr,
		},
	}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "3333"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)

	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()

	select {
	case <-sessionRegistered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("Timeout: session was not registered")
	}

	select {
	case <-writeAttempted:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("Timeout: expected write to be attempted")
	}

	<-done

	if len(writer.wrote) != 0 {
		t.Errorf("expected write to fail and no data to be written, but got: %x", writer.wrote)
	}
}

func TestTransportHandler_HappyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}

	clientAddr := netip.MustParseAddrPort("192.168.1.50:5050")
	internalIP := netip.MustParseAddr("10.0.0.50")
	sessionRepo := &testSessionRepo{
		sessions: map[netip.AddrPort]connection.Session{},
	}
	fakeCrypto := &alwaysWriteCrypto{}
	sessionRepo.sessions[clientAddr] = Session{
		crypto:     fakeCrypto,
		internalIP: internalIP,
		externalIP: clientAddr,
	}

	fakeHS := &fakeHandshake{ip: internalIP.AsSlice()}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xde, 0xad, 0xbe, 0xef}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "5050"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	if len(writer.wrote) != 1 {
		t.Fatalf("expected 1 packet to be written to TUN, got %d", len(writer.wrote))
	}
}

func TestTransportHandler_NATRebinding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}

	oldAddr := netip.MustParseAddrPort("192.168.1.51:5050")
	newAddr := netip.MustParseAddrPort("192.168.1.51:6060")
	internalIP := netip.MustParseAddr("10.0.0.51")

	sessionRepo := &testSessionRepo{sessions: map[netip.AddrPort]connection.Session{}}
	fakeCrypto := &alwaysWriteCrypto{}
	sessionRepo.sessions[oldAddr] = Session{
		crypto:     fakeCrypto,
		internalIP: internalIP,
		externalIP: oldAddr,
	}

	sessionRegistered := make(chan struct{})
	sessionRepo.afterAdd = func() { close(sessionRegistered) }

	fakeHS := &fakeHandshake{ip: internalIP.AsSlice()}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xca, 0xfe}},
		readAddrs: []netip.AddrPort{newAddr},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "6060"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	select {
	case <-sessionRegistered:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Timeout: session was not re-registered")
	}
	<-done
}

func TestTransportHandler_RegisterClient_BadInternalIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRepo := &testSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.60:6000")

	// Handshake returns invalid data (incorrect IP address)
	badIP := []byte{1, 2, 3}
	fakeHS := &fakeHandshake{ip: badIP}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}
	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0x01}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "6000"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	if len(sessionRepo.adds) != 0 {
		t.Errorf("expected no session registered due to bad internalIP, got %d", len(sessionRepo.adds))
	}
}

func TestTransportHandler_ErrorSetBuffer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRepo := &testSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.70:7000")
	internalIP := net.ParseIP("10.0.0.70")
	fakeHS := &fakeHandshake{ip: internalIP}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xbe, 0xef}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "7000"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
		&servicePacketMock{},
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done
}

func Test_fakeAEAD_DoesNotUseRand(t *testing.T) {
	var f fakeAEAD
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

func TestTransportHandler_HandshakeError_ServicePacketEncodeError_NoSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRepo := &testSessionRepo{}
	clientAddr := netip.MustParseAddrPort("192.168.1.99:5999")

	fakeHS := &fakeHandshake{ip: nil, err: errors.New("hs fail")}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xab, 0xcd}},
		readAddrs: []netip.AddrPort{clientAddr},
	}

	sp := &servicePacketEncodeErrMock{called: make(chan struct{}, 1)}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "5999"},
		writer,
		conn,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(mockAEADBuilder{}),
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

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not exit after cancel")
	}

	if got := len(conn.writes); got != 0 {
		t.Fatalf("expected no UDP writes on EncodeLegacy error, got %d", got)
	}
}
