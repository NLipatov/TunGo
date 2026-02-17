package tcp_registration

import (
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/session"
)

// tcpRegLogger is a no-op logger for tests.
type tcpRegLogger struct{}

func (tcpRegLogger) Printf(string, ...any) {}

// tcpRegHandshake is a mock handshake.
type tcpRegHandshake struct {
	clientID int
	id       [32]byte
	c2s, s2c []byte
	err      error
}

func (h *tcpRegHandshake) Id() [32]byte              { return h.id }
func (h *tcpRegHandshake) KeyClientToServer() []byte { return h.c2s }
func (h *tcpRegHandshake) KeyServerToClient() []byte { return h.s2c }
func (*tcpRegHandshake) ClientSideHandshake(_ connection.Transport) error {
	return nil
}
func (h *tcpRegHandshake) ServerSideHandshake(_ connection.Transport) (int, error) {
	if h.err != nil {
		return 0, h.err
	}
	return h.clientID, nil
}

// tcpRegHandshakeFactory returns a pre-configured handshake.
type tcpRegHandshakeFactory struct {
	handshake *tcpRegHandshake
}

func (f *tcpRegHandshakeFactory) NewHandshake() connection.Handshake {
	return f.handshake
}

// tcpRegCrypto is a mock crypto.
type tcpRegCrypto struct{}

func (tcpRegCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (tcpRegCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

// tcpRegRekeyer is a mock rekeyer.
type tcpRegRekeyer struct{}

func (tcpRegRekeyer) Rekey(_, _ []byte) (uint16, error) { return 0, nil }
func (tcpRegRekeyer) SetSendEpoch(uint16)               {}
func (tcpRegRekeyer) RemoveEpoch(uint16) bool           { return true }

// tcpRegCryptoFactory returns a pre-configured crypto.
type tcpRegCryptoFactory struct {
	crypto connection.Crypto
	ctrl   *rekey.StateMachine
	err    error
}

func (f *tcpRegCryptoFactory) FromHandshake(connection.Handshake, bool) (connection.Crypto, *rekey.StateMachine, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.crypto, f.ctrl, nil
}

// tcpRegConn is a mock net.Conn backed by a pipe.
type tcpRegConn struct {
	net.Conn
	remoteAddr net.Addr
	closed     bool
}

func (c *tcpRegConn) RemoteAddr() net.Addr { return c.remoteAddr }
func (c *tcpRegConn) Close() error         { c.closed = true; return nil }
func (*tcpRegConn) Read(_ []byte) (int, error) {
	// Simulate a blocking read for the framing adapter.
	time.Sleep(time.Millisecond)
	return 0, errors.New("read error")
}
func (*tcpRegConn) Write(b []byte) (int, error)      { return len(b), nil }
func (*tcpRegConn) SetDeadline(time.Time) error      { return nil }
func (*tcpRegConn) SetReadDeadline(time.Time) error  { return nil }
func (*tcpRegConn) SetWriteDeadline(time.Time) error { return nil }
func (*tcpRegConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
}

func TestNewRegistrar(t *testing.T) {
	r := NewRegistrar(tcpRegLogger{}, nil, nil, nil, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	if r == nil {
		t.Fatal("expected non-nil registrar")
	}
}

func TestRegisterClient_HandshakeError_ClosesConn(t *testing.T) {
	hsErr := errors.New("handshake failed")
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{err: hsErr},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	peer, transport, err := reg.RegisterClient(conn)
	if err == nil {
		t.Fatal("expected error from handshake failure")
	}
	if peer != nil || transport != nil {
		t.Fatal("expected nil peer and transport on error")
	}
	if !conn.closed {
		t.Fatal("expected conn to be closed on handshake failure")
	}
}

func TestRegisterClient_CryptoFactoryError_ClosesConn(t *testing.T) {
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cryptoErr := errors.New("crypto init failed")
	cf := &tcpRegCryptoFactory{err: cryptoErr}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	peer, transport, err := reg.RegisterClient(conn)
	if err == nil {
		t.Fatal("expected error from crypto factory failure")
	}
	if peer != nil || transport != nil {
		t.Fatal("expected nil peer and transport on error")
	}
	if !conn.closed {
		t.Fatal("expected conn to be closed on crypto failure")
	}
}

func TestRegisterClient_Success(t *testing.T) {
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	peer, transport, err := reg.RegisterClient(conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peer == nil {
		t.Fatal("expected non-nil peer")
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}

	// Verify peer is in repo. AllocateClientIP(10.0.0.0/24, 1) → 10.0.0.2
	ip := netip.MustParseAddr("10.0.0.2")
	found, findErr := repo.GetByInternalAddrPort(ip)
	if findErr != nil {
		t.Fatalf("expected peer in repo, got error: %v", findErr)
	}
	if found != peer {
		t.Fatal("expected same peer in repo")
	}
}

// tcpRegEgress tracks Close calls.
type tcpRegEgress struct {
	closed bool
}

func (*tcpRegEgress) SendDataIP([]byte) error  { return nil }
func (*tcpRegEgress) SendControl([]byte) error { return nil }
func (e *tcpRegEgress) Close() error           { e.closed = true; return nil }

func TestRegisterClient_ReplacesExistingSession(t *testing.T) {
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()

	// Pre-populate repo with an existing session for the same internal IP.
	// AllocateClientIP(10.0.0.0/24, 1) → 10.0.0.2
	ip := netip.MustParseAddr("10.0.0.2")
	existingSession := session.NewSession(tcpRegCrypto{}, nil, ip, netip.MustParseAddrPort("192.168.1.100:9999"))
	oldEgress := &tcpRegEgress{}
	existingPeer := session.NewPeer(existingSession, oldEgress)
	repo.Add(existingPeer)

	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	// RegisterClient should replace the existing session.
	peer, _, err := reg.RegisterClient(conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peer == nil {
		t.Fatal("expected non-nil peer")
	}

	// Old peer's egress should be closed.
	if !oldEgress.closed {
		t.Fatal("expected old peer's egress to be closed")
	}
	// New conn must NOT be closed — it's the active connection.
	if conn.closed {
		t.Fatal("new conn must not be closed when replacing existing session")
	}

	// Old peer should be gone — new peer should be found.
	found, findErr := repo.GetByInternalAddrPort(ip)
	if findErr != nil {
		t.Fatalf("expected peer in repo: %v", findErr)
	}
	if found == existingPeer {
		t.Fatal("expected existing peer to be replaced")
	}
}

func TestRegisterClient_NonTCPAddr_ClosesConn(t *testing.T) {
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	// Use a UDP address instead of TCP.
	conn := &tcpRegConn{
		remoteAddr: &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	_, _, err := reg.RegisterClient(conn)
	if err == nil {
		t.Fatal("expected error for non-TCP remote address")
	}
	if !conn.closed {
		t.Fatal("expected conn to be closed")
	}
}

// tcpRegFailingRepo returns a non-ErrNotFound error from GetByInternalAddrPort.
type tcpRegFailingRepo struct {
	err error
}

func (*tcpRegFailingRepo) Add(*session.Peer)    {}
func (*tcpRegFailingRepo) Delete(*session.Peer) {}
func (r *tcpRegFailingRepo) GetByInternalAddrPort(netip.Addr) (*session.Peer, error) {
	return nil, r.err
}
func (r *tcpRegFailingRepo) GetByExternalAddrPort(netip.AddrPort) (*session.Peer, error) {
	return nil, r.err
}
func (r *tcpRegFailingRepo) GetByRouteID(uint64) (*session.Peer, error) { return nil, r.err }
func (r *tcpRegFailingRepo) FindByDestinationIP(netip.Addr) (*session.Peer, error) {
	return nil, r.err
}
func (*tcpRegFailingRepo) AllPeers() []*session.Peer                            { return nil }
func (*tcpRegFailingRepo) UpdateExternalAddr(_ *session.Peer, _ netip.AddrPort) {}

func TestRegisterClient_LookupError_ClosesConn(t *testing.T) {
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := &tcpRegFailingRepo{err: errors.New("database unavailable")}
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	peer, transport, err := reg.RegisterClient(conn)
	if err == nil {
		t.Fatal("expected error from lookup failure")
	}
	if peer != nil || transport != nil {
		t.Fatal("expected nil peer and transport on lookup error")
	}
	if !conn.closed {
		t.Fatal("expected conn to be closed on lookup error")
	}
}

func TestRegisterClient_NegativeClientID_FailsAllocation(t *testing.T) {
	// Negative clientID causes AllocateClientIP to fail.
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{
			clientID: -1, // invalid
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	peer, transport, err := reg.RegisterClient(conn)
	if err == nil {
		t.Fatal("expected error from IP allocation with negative clientID")
	}
	if peer != nil || transport != nil {
		t.Fatal("expected nil peer and transport on allocation error")
	}
}

// tcpRegHandshakeResult implements connection.HandshakeResult.
type tcpRegHandshakeResult struct {
	pubKey     []byte
	allowedIPs []netip.Prefix
}

func (r *tcpRegHandshakeResult) ClientPubKey() []byte       { return r.pubKey }
func (r *tcpRegHandshakeResult) AllowedIPs() []netip.Prefix { return r.allowedIPs }

// tcpRegHandshakeWithResult extends tcpRegHandshake with HandshakeWithResult.
type tcpRegHandshakeWithResult struct {
	tcpRegHandshake
	result connection.HandshakeResult
}

func (h *tcpRegHandshakeWithResult) Result() connection.HandshakeResult { return h.result }

// tcpRegHandshakeWithResultFactory returns a HandshakeWithResult mock.
type tcpRegHandshakeWithResultFactory struct {
	handshake *tcpRegHandshakeWithResult
}

func (f *tcpRegHandshakeWithResultFactory) NewHandshake() connection.Handshake {
	return f.handshake
}

// tcpRegCookieHandshakeFactory returns ErrCookieRequired on the first
// NewHandshake call, then a successful handshake on the second.
type tcpRegCookieHandshakeFactory struct {
	calls    int
	clientID int
}

func (f *tcpRegCookieHandshakeFactory) NewHandshake() connection.Handshake {
	f.calls++
	if f.calls == 1 {
		return &tcpRegHandshake{err: noise.ErrCookieRequired}
	}
	return &tcpRegHandshake{
		clientID: f.clientID,
		c2s:      make([]byte, 32),
		s2c:      make([]byte, 32),
	}
}

func TestRegisterClient_CookieRetry_Success(t *testing.T) {
	hf := &tcpRegCookieHandshakeFactory{clientID: 1}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	peer, transport, err := reg.RegisterClient(conn)
	if err != nil {
		t.Fatalf("expected success after cookie retry, got: %v", err)
	}
	if peer == nil || transport == nil {
		t.Fatal("expected non-nil peer and transport")
	}
	if conn.closed {
		t.Fatal("connection must not be closed on successful retry")
	}
	if hf.calls != 2 {
		t.Fatalf("expected 2 handshake attempts (cookie + retry), got %d", hf.calls)
	}
}

func TestRegisterClient_CookieRetry_SecondFailure_ClosesConn(t *testing.T) {
	// Both attempts return ErrCookieRequired — second should close.
	hf := &tcpRegHandshakeFactory{
		handshake: &tcpRegHandshake{err: noise.ErrCookieRequired},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	_, _, err := reg.RegisterClient(conn)
	if err == nil {
		t.Fatal("expected error when cookie retry also fails")
	}
	if !conn.closed {
		t.Fatal("expected conn to be closed after second cookie failure")
	}
}

func TestRegisterClient_HandshakeWithResult_IPv6(t *testing.T) {
	hf := &tcpRegHandshakeWithResultFactory{
		handshake: &tcpRegHandshakeWithResult{
			tcpRegHandshake: tcpRegHandshake{
				clientID: 1,
				c2s:      make([]byte, 32),
				s2c:      make([]byte, 32),
			},
			result: &tcpRegHandshakeResult{
				pubKey:     []byte("test-client-pub-key-32-bytes!!!!"),
				allowedIPs: []netip.Prefix{netip.MustParsePrefix("192.168.100.0/24")},
			},
		},
	}
	cf := &tcpRegCryptoFactory{
		crypto: tcpRegCrypto{},
		ctrl:   rekey.NewStateMachine(tcpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}
	repo := session.NewDefaultRepository()
	reg := NewRegistrar(tcpRegLogger{}, hf, cf, repo,
		netip.MustParsePrefix("10.0.0.0/24"),
		netip.MustParsePrefix("fd00::/64"),
	)

	conn := &tcpRegConn{
		remoteAddr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 12345},
	}

	peer, transport, err := reg.RegisterClient(conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peer == nil || transport == nil {
		t.Fatal("expected non-nil peer and transport")
	}

	// HandshakeWithResult allowedIPs (192.168.100.0/24) should be applied.
	if !peer.IsSourceAllowed(netip.MustParseAddr("192.168.100.5")) {
		t.Fatal("expected allowedIPs from handshake result to be applied")
	}

	// IPv6 allocation (fd00::2 for clientID=1) should be in allowedAddrs.
	if !peer.IsSourceAllowed(netip.MustParseAddr("fd00::2")) {
		t.Fatal("expected IPv6 address to be allowed")
	}
}
