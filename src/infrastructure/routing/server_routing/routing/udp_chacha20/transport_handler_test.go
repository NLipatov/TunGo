package udp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/settings"
)

type errorBufferConn struct {
	fakeUdpListenerConn
}

func (e *errorBufferConn) SetReadBuffer(_ int) error  { return errors.New("fail set read buf") }
func (e *errorBufferConn) SetWriteBuffer(_ int) error { return errors.New("fail set write buf") }

type alwaysWriteCrypto struct{}

func (d *alwaysWriteCrypto) Encrypt(in []byte) ([]byte, error) { return in, nil }
func (d *alwaysWriteCrypto) Decrypt(in []byte) ([]byte, error) { return in, nil }

type fakeCryptographyService struct {
	encErr, decErr error
	enc, dec       []byte
}

func (f *fakeCryptographyService) Encrypt(in []byte) ([]byte, error) {
	if f.encErr != nil {
		return nil, f.encErr
	}
	if f.enc != nil {
		return f.enc, nil
	}
	return in, nil
}
func (f *fakeCryptographyService) Decrypt(in []byte) ([]byte, error) {
	if f.decErr != nil {
		return nil, f.decErr
	}
	if f.dec != nil {
		return f.dec, nil
	}
	return in, nil
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

type fakeListener struct {
	conn application.UdpListenerConn
	err  error
}

func (f *fakeListener) Listen() (application.UdpListenerConn, error) { return f.conn, f.err }
func (f *fakeListener) Read(_, _ []byte) (int, int, int, netip.AddrPort, error) {
	return 0, 0, 0, netip.AddrPort{}, nil
}

type fakeWriter struct {
	buf     bytes.Buffer
	err     error
	wrote   [][]byte
	writeCh chan struct{}
}

func (f *fakeWriter) Write(p []byte) (int, error) {
	f.wrote = append(f.wrote, append([]byte(nil), p...))
	if f.writeCh != nil {
		select {
		case f.writeCh <- struct{}{}:
		default:
		}
	}
	if f.err != nil {
		return 0, f.err
	}
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

func (f *fakeHandshake) Id() [32]byte      { return f.id }
func (f *fakeHandshake) ClientKey() []byte { return f.client[:] }
func (f *fakeHandshake) ServerKey() []byte { return f.server[:] }
func (f *fakeHandshake) ServerSideHandshake(_ application.ConnectionAdapter) (net.IP, error) {
	return f.ip, f.err
}
func (f *fakeHandshake) ClientSideHandshake(_ application.ConnectionAdapter, _ settings.Settings) error {
	return nil
}

type fakeHandshakeFactory struct {
	hs application.Handshake
}

func (f *fakeHandshakeFactory) NewHandshake() application.Handshake { return f.hs }

type testSessionRepo struct {
	sessions map[netip.AddrPort]Session
	adds     []Session
	afterAdd func()
}

func (r *testSessionRepo) Add(s Session) {
	if r.sessions == nil {
		r.sessions = map[netip.AddrPort]Session{}
	}
	r.sessions[s.remoteAddrPort] = s
	r.adds = append(r.adds, s)
	if r.afterAdd != nil {
		r.afterAdd()
	}
}
func (r *testSessionRepo) Delete(_ Session) {}
func (r *testSessionRepo) GetByInternalAddrPort(_ netip.Addr) (Session, error) {
	return Session{}, errors.New("not implemented")
}
func (r *testSessionRepo) GetByExternalAddrPort(addr netip.AddrPort) (Session, error) {
	s, ok := r.sessions[addr]
	if !ok {
		return Session{}, errors.New("no session")
	}
	return s, nil
}

func (r *testSessionRepo) Range(f func(s Session) bool) {
	for _, session := range r.sessions {
		if !f(session) {
			break
		}
	}
}

func TestTransportHandler_RegistrationPacket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}
	sessionRegistered := make(chan struct{})

	sessionRepo := &testSessionRepo{
		afterAdd: func() {
			close(sessionRegistered)
		},
	}

	clientAddr := netip.MustParseAddrPort("192.168.1.10:5555")
	internalIP := net.ParseIP("10.0.0.5")
	fakeHS := &fakeHandshake{ip: internalIP}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xde, 0xad}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	listener := &fakeListener{conn: conn}

	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "9999"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
	)

	go func() {
		_ = handler.HandleTransport()
	}()

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
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}
	writeCh := make(chan struct{}, 1)
	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xab, 0xcd}},
		readAddrs: []netip.AddrPort{clientAddr},
		writeCh:   writeCh,
	}
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "1111"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
	)
	done := make(chan struct{})
	go func() {
		_ = handler.HandleTransport()
		close(done)
	}()
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
		readBufs:  [][]byte{{0x01, 0x02}, {0x03, 0x04}},
		readAddrs: []netip.AddrPort{clientAddr, clientAddr},
	}
	fakeCrypto := &struct{ fakeCryptographyService }{fakeCryptographyService{decErr: errors.New("fail decrypt")}}
	sessionRegistered := make(chan struct{})
	sessionRepo.afterAdd = func() {
		s := sessionRepo.adds[0]
		s.CryptographyService = fakeCrypto
		sessionRepo.sessions[clientAddr] = s
		close(sessionRegistered)
	}
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "2222"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
	)
	done := make(chan struct{})
	go func() {
		_ = handler.HandleTransport()
		close(done)
	}()
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
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "4444"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
	)
	done := make(chan struct{})
	go func() {
		_ = handler.HandleTransport()
		close(done)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done
}

func TestTransportHandler_WriteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeCh := make(chan struct{}, 1)
	writer := &fakeWriter{err: errors.New("write fail"), writeCh: writeCh}
	logger := &fakeLogger{}
	clientAddr := netip.MustParseAddrPort("192.168.1.40:6000")
	internalIP := net.ParseIP("10.0.0.40")
	fakeHS := &fakeHandshake{ip: internalIP}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}

	sessionRepo := &testSessionRepo{}
	sessionRegistered := make(chan struct{})
	sessionRepo.afterAdd = func() {
		s := sessionRepo.adds[0]
		s.CryptographyService = &alwaysWriteCrypto{}
		sessionRepo.sessions[clientAddr] = s
		close(sessionRegistered)
	}

	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0xaa, 0xbb}, {0xcc, 0xdd}},
		readAddrs: []netip.AddrPort{clientAddr, clientAddr},
	}
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "3333"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
	)

	done := make(chan struct{})
	go func() {
		_ = handler.HandleTransport()
		close(done)
	}()

	<-sessionRegistered

	select {
	case <-writeCh:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("expected write to be attempted")
	}
	<-done
}

func TestTransportHandler_HappyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writer := &fakeWriter{}
	logger := &fakeLogger{}

	clientAddr := netip.MustParseAddrPort("192.168.1.50:5050")
	internalIP := netip.MustParseAddr("10.0.0.50")
	sessionRepo := &testSessionRepo{
		sessions: map[netip.AddrPort]Session{},
	}
	fakeCrypto := &alwaysWriteCrypto{}
	sessionRepo.sessions[clientAddr] = Session{
		remoteAddrPort:      clientAddr,
		CryptographyService: fakeCrypto,
		internalIP:          internalIP,
		externalIP:          clientAddr,
	}

	fakeHS := &fakeHandshake{ip: internalIP.AsSlice()}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}
	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0x01, 0x02, 0x03}},
		readAddrs: []netip.AddrPort{clientAddr},
	}
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "5050"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
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

	// Old address
	oldAddr := netip.MustParseAddrPort("192.168.1.51:5050")
	// New address
	newAddr := netip.MustParseAddrPort("192.168.1.51:6060")
	internalIP := netip.MustParseAddr("10.0.0.51")

	sessionRepo := &testSessionRepo{
		sessions: map[netip.AddrPort]Session{},
	}
	fakeCrypto := &alwaysWriteCrypto{}
	// Existing session, old address
	sessionRepo.sessions[oldAddr] = Session{
		remoteAddrPort:      oldAddr,
		CryptographyService: fakeCrypto,
		internalIP:          internalIP,
		externalIP:          oldAddr,
	}
	// Tracking new session registration
	sessionRegistered := make(chan struct{})
	sessionRepo.afterAdd = func() { close(sessionRegistered) }

	fakeHS := &fakeHandshake{ip: internalIP.AsSlice()}
	handshakeFactory := &fakeHandshakeFactory{hs: fakeHS}
	conn := &fakeUdpListenerConn{
		readBufs:  [][]byte{{0x01, 0x02, 0x03}},
		readAddrs: []netip.AddrPort{newAddr},
	}
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "6060"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
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
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "6000"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	// Check that invalid session was not added
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

	conn := &errorBufferConn{
		fakeUdpListenerConn{
			readBufs:  [][]byte{{0x77, 0x88}},
			readAddrs: []netip.AddrPort{clientAddr},
		},
	}
	listener := &fakeListener{conn: conn}
	handler := NewTransportHandler(
		ctx,
		settings.Settings{Port: "7000"},
		writer,
		listener,
		sessionRepo,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(),
	)
	done := make(chan struct{})
	go func() { _ = handler.HandleTransport(); close(done) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done
}
