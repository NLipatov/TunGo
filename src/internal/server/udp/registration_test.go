package udp

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"sync"
	"testing"
	"time"
	"tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/chacha20/rekey"
	"tungo/internal/protocol/noise"
	tunnelrekey "tungo/internal/protocol/rekey"
	"tungo/internal/server/session"
)

// udpRegEpochManager is a mock epoch manager.
type udpRegEpochManager struct{}

func (udpRegEpochManager) StageEpoch(_, _ []byte) (uint16, error) { return 0, nil }
func (udpRegEpochManager) PromoteSendEpoch(uint16)                {}
func (udpRegEpochManager) RetirePreviousEpoch() bool              { return true }

// udpRegCrypto is a mock crypto.
type udpRegCrypto struct{}

func (udpRegCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (udpRegCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

// udpRegHandshake is a mock handshake that reads from registration queue.
type udpRegHandshake struct {
	clientID int
	err      error
	id       [32]byte
	c2s, s2c []byte
}

func (h *udpRegHandshake) ID() [32]byte              { return h.id }
func (h *udpRegHandshake) KeyClientToServer() []byte { return h.c2s }
func (h *udpRegHandshake) KeyServerToClient() []byte { return h.s2c }
func (h *udpRegHandshake) ClientSideHandshake(_ io.ReadWriter) error {
	return nil
}
func (h *udpRegHandshake) ServerSideHandshake(transport io.ReadWriter) (int, error) {
	// Simulate handshake: read from queue, write response.
	buf := make([]byte, 1024)
	if _, err := transport.Read(buf); err != nil {
		return 0, err
	}
	if _, err := transport.Write([]byte("ok")); err != nil {
		return 0, err
	}
	if h.err != nil {
		return 0, h.err
	}
	return h.clientID, nil
}

type udpRegHandshakeFactory struct {
	handshake *udpRegHandshake
}

func (f *udpRegHandshakeFactory) NewHandshake() handshake {
	return f.handshake
}

type udpRegCryptoFactory struct {
	crypto crypto
	ctrl   epochController
	err    error
}

func (f *udpRegCryptoFactory) FromHandshake(chacha20.KeyMaterial, bool) (crypto, epochController, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.crypto, f.ctrl, nil
}

// udpRegListener is a minimal mock for UdpListener.
type udpRegListener struct {
	mu      sync.Mutex
	written []udpRegWrite
}

type udpRegWrite struct {
	data []byte
	addr netip.AddrPort
}

func (l *udpRegListener) Close() error             { return nil }
func (l *udpRegListener) SetReadBuffer(int) error  { return nil }
func (l *udpRegListener) SetWriteBuffer(int) error { return nil }
func (l *udpRegListener) ReadMsgUDPAddrPort(_, _ []byte) (int, int, int, netip.AddrPort, error) {
	return 0, 0, 0, netip.AddrPort{}, errors.New("not implemented")
}

func (l *udpRegListener) WriteToUDPAddrPort(data []byte, addr netip.AddrPort) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	buf := make([]byte, len(data))
	copy(buf, data)
	l.written = append(l.written, udpRegWrite{data: buf, addr: addr})
	return len(data), nil
}

func registrationCount(r *registrar) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.registrations)
}

func TestNewRegistrar_CreatesEmptyRegistrations(t *testing.T) {
	ctx := context.Background()
	r := newRegistrar(ctx, nil, nil, nil, nil, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	if r == nil {
		t.Fatal("expected non-nil registrar")
	}
	if registrationCount(r) != 0 {
		t.Fatal("expected empty registrations map")
	}
}

func TestGetOrCreateRegistrationQueue_CreatesNew(t *testing.T) {
	ctx := context.Background()
	r := newRegistrar(ctx, nil, nil, nil, nil, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, isNew := r.getOrCreateRegistrationQueue(addr)
	if !isNew {
		t.Fatal("expected isNew=true for first call")
	}
	if q == nil {
		t.Fatal("expected non-nil queue")
	}

	// Second call returns existing.
	q2, isNew2 := r.getOrCreateRegistrationQueue(addr)
	if isNew2 {
		t.Fatal("expected isNew=false for second call")
	}
	if q2 != q {
		t.Fatal("expected same queue on second call")
	}
}

func TestCloseAll_ClearsRegistrations(t *testing.T) {
	ctx := context.Background()
	r := newRegistrar(ctx, nil, nil, nil, nil, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	r.getOrCreateRegistrationQueue(netip.MustParseAddrPort("192.168.1.1:1234"))
	r.getOrCreateRegistrationQueue(netip.MustParseAddrPort("192.168.1.2:5678"))

	if registrationCount(r) != 2 {
		t.Fatalf("expected 2 registrations, got %d", registrationCount(r))
	}

	r.closeAll()

	if registrationCount(r) != 0 {
		t.Fatalf("expected 0 registrations after CloseAll, got %d", registrationCount(r))
	}
}

func TestEnqueuePacket_CreatesQueueAndStartsRegistration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			err: errors.New("handshake fail"), // cause registration to fail
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegEpochManager{}, []byte("c2s"), []byte("s2c")),
	}

	r := newRegistrar(ctx, listener, repo, hf.NewHandshake, cf.FromHandshake, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	r.enqueuePacket(addr, []byte("hello"))

	// After registration failure, the queue should be removed.
	// Give time for the registration goroutine to complete.
	time.Sleep(100 * time.Millisecond)
	if registrationCount(r) != 0 {
		t.Fatalf("expected registration removed after failure, got %d", registrationCount(r))
	}
}

func TestRegisterClient_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegEpochManager{}, []byte("c2s"), []byte("s2c")),
	}

	r := newRegistrar(ctx, listener, repo, hf.NewHandshake, cf.FromHandshake, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.getOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.registerClient(addr, q)
		close(done)
	}()

	// Feed the handshake a packet (simulate client hello).
	q.enqueue([]byte("client-hello"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RegisterClient to complete")
	}

	// Verify session was added.
	ip := netip.MustParseAddr("10.0.0.2") // AllocateClientIP(10.0.0.0/24, 1) → 10.0.0.2
	peer, err := repo.GetByInternalAddrPort(ip)
	if err != nil {
		t.Fatalf("expected peer in repo: %v", err)
	}
	if peer == nil {
		t.Fatal("expected non-nil peer")
	}
}

func TestRegisterClient_CryptoFactoryError_FailsGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &udpRegCryptoFactory{err: errors.New("crypto failed")}

	r := newRegistrar(ctx, listener, repo, hf.NewHandshake, cf.FromHandshake, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.getOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.registerClient(addr, q)
		close(done)
	}()

	q.enqueue([]byte("client-hello"))

	select {
	case <-done:
		// Registration completed (with failure)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RegisterClient to complete")
	}

	// No session should have been added due to crypto error
	ip := netip.MustParseAddr("10.0.0.2")
	_, err := repo.GetByInternalAddrPort(ip)
	if err == nil {
		t.Fatal("expected no session in repo after crypto error")
	}
}

func TestRegisterClient_NegativeClientID_FailsAllocation(t *testing.T) {
	// Negative clientID causes AllocateClientIP to fail.
	// UDP registrar logs and returns silently, no session added.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			clientID: -1, // invalid
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegEpochManager{}, []byte("c2s"), []byte("s2c")),
	}

	r := newRegistrar(ctx, listener, repo, hf.NewHandshake, cf.FromHandshake, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.getOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.registerClient(addr, q)
		close(done)
	}()

	q.enqueue([]byte("client-hello"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RegisterClient to complete")
	}

	// No session should be added due to allocation failure.
	_, err := repo.GetByInternalAddrPort(netip.MustParseAddr("10.0.0.1"))
	if err == nil {
		t.Fatal("expected no session in repo after allocation failure")
	}
}

type udpRegHandshakeResult struct {
	pubKey     []byte
	allowedIPs []netip.Prefix
}

func (r *udpRegHandshakeResult) ClientPubKey() []byte       { return r.pubKey }
func (r *udpRegHandshakeResult) AllowedIPs() []netip.Prefix { return r.allowedIPs }

// udpRegHandshakeWithResult extends udpRegHandshake with HandshakeWithResult.
type udpRegHandshakeWithResult struct {
	udpRegHandshake
	result *udpRegHandshakeResult
}

func (h *udpRegHandshakeWithResult) ClientPubKey() []byte       { return h.result.ClientPubKey() }
func (h *udpRegHandshakeWithResult) AllowedIPs() []netip.Prefix { return h.result.AllowedIPs() }

type udpRegHandshakeWithResultFactory struct {
	handshake *udpRegHandshakeWithResult
}

func (f *udpRegHandshakeWithResultFactory) NewHandshake() handshake {
	return f.handshake
}

func TestEnqueuePacket_AtCapacity_SilentDrop(t *testing.T) {
	ctx := context.Background()
	r := newRegistrar(ctx, nil, nil, nil, nil, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	// Fill up to maxConcurrentRegistrations using direct queue creation.
	for i := 0; i < maxConcurrentRegistrations; i++ {
		ip := netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)})
		addr := netip.AddrPortFrom(ip, 1234)
		r.getOrCreateRegistrationQueue(addr)
	}

	if registrationCount(r) != maxConcurrentRegistrations {
		t.Fatalf("expected %d registrations, got %d", maxConcurrentRegistrations, registrationCount(r))
	}

	// EnqueuePacket for a new address should be silently dropped.
	excess := netip.MustParseAddrPort("255.255.255.255:9999")
	r.enqueuePacket(excess, []byte("should-be-dropped"))

	// No new queue should have been created.
	if registrationCount(r) != maxConcurrentRegistrations {
		t.Fatalf("expected %d registrations after drop, got %d", maxConcurrentRegistrations, registrationCount(r))
	}
}

func TestRegisterClient_HandshakeWithResult_IPv6(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewRepository()

	hf := &udpRegHandshakeWithResultFactory{
		handshake: &udpRegHandshakeWithResult{
			udpRegHandshake: udpRegHandshake{
				clientID: 1,
				c2s:      make([]byte, 32),
				s2c:      make([]byte, 32),
			},
			result: &udpRegHandshakeResult{
				pubKey:     []byte("test-client-pub-key-32-bytes!!!!"),
				allowedIPs: []netip.Prefix{netip.MustParsePrefix("192.168.100.0/24")},
			},
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegEpochManager{}, []byte("c2s"), []byte("s2c")),
	}

	r := newRegistrar(ctx, listener, repo, hf.NewHandshake, cf.FromHandshake,
		netip.MustParsePrefix("10.0.0.0/24"),
		netip.MustParsePrefix("fd00::/64"),
	)

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.getOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.registerClient(addr, q)
		close(done)
	}()

	q.enqueue([]byte("client-hello"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RegisterClient to complete")
	}

	// Verify session has auth data.
	ip := netip.MustParseAddr("10.0.0.2")
	peer, err := repo.GetByInternalAddrPort(ip)
	if err != nil {
		t.Fatalf("expected peer in repo: %v", err)
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

// udpRegCookieHandshake returns ErrCookieRequired (after reading from transport)
// so the retry loop creates a fresh handshake.
type udpRegCookieHandshake struct {
	udpRegHandshake
}

func (h *udpRegCookieHandshake) ServerSideHandshake(transport io.ReadWriter) (int, error) {
	buf := make([]byte, 1024)
	if _, err := transport.Read(buf); err != nil {
		return 0, err
	}
	if _, err := transport.Write([]byte("cookie")); err != nil {
		return 0, err
	}
	return 0, noise.ErrCookieRequired
}

// udpRegCookieHandshakeFactory returns a cookie handshake first, then a
// successful handshake on the second call.
type udpRegCookieHandshakeFactory struct {
	calls    int
	clientID int
}

func (f *udpRegCookieHandshakeFactory) NewHandshake() handshake {
	f.calls++
	if f.calls == 1 {
		return &udpRegCookieHandshake{}
	}
	return &udpRegHandshake{
		clientID: f.clientID,
		c2s:      make([]byte, 32),
		s2c:      make([]byte, 32),
	}
}

func TestRegisterClient_CookieRetry_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewRepository()

	hf := &udpRegCookieHandshakeFactory{clientID: 1}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegEpochManager{}, []byte("c2s"), []byte("s2c")),
	}

	r := newRegistrar(ctx, listener, repo, hf.NewHandshake, cf.FromHandshake, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.getOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.registerClient(addr, q)
		close(done)
	}()

	// First packet triggers cookie response, second packet is the retry.
	q.enqueue([]byte("client-hello"))
	q.enqueue([]byte("client-hello-with-cookie"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RegisterClient to complete")
	}

	// Verify session was added after cookie retry.
	ip := netip.MustParseAddr("10.0.0.2")
	peer, err := repo.GetByInternalAddrPort(ip)
	if err != nil {
		t.Fatalf("expected peer in repo after cookie retry: %v", err)
	}
	if peer == nil {
		t.Fatal("expected non-nil peer")
	}
	if hf.calls != 2 {
		t.Fatalf("expected 2 handshake attempts (cookie + retry), got %d", hf.calls)
	}
}

func TestRegisterClient_ReplacesExistingSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewRepository()
	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			clientID: 1,
			c2s:      make([]byte, 32),
			s2c:      make([]byte, 32),
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegEpochManager{}, []byte("c2s"), []byte("s2c")),
	}

	r := newRegistrar(ctx, listener, repo, hf.NewHandshake, cf.FromHandshake, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})
	internalIP := netip.MustParseAddr("10.0.0.2")
	staleRekey := tunnelrekey.NewServerRekeyCoordinator(
		rekey.NewStateMachine(udpRegEpochManager{}, []byte("old-c2s"), []byte("old-s2c")),
		nil,
	)
	repo.Add(session.NewPeerWithAuth(
		udpRegCrypto{}, staleRekey, internalIP, netip.MustParseAddrPort("192.168.1.10:1234"), nil, nil,
		io.Discard,
	))

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.getOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.registerClient(addr, q)
		close(done)
	}()

	q.enqueue([]byte("client-hello"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RegisterClient to complete")
	}

	peer, err := repo.GetByInternalAddrPort(internalIP)
	if err != nil {
		t.Fatalf("expected replacement peer in repo: %v", err)
	}
	if peer == nil {
		t.Fatal("expected non-nil replacement peer")
	}

}

func TestGetOrCreateRegistrationQueue_SecondCallReusesQueue(t *testing.T) {
	ctx := context.Background()
	r := newRegistrar(ctx, nil, nil, nil, nil, netip.MustParsePrefix("10.0.0.0/24"), netip.Prefix{})

	addr := netip.MustParseAddrPort("192.168.1.1:1234")

	q1, isNew1 := r.getOrCreateRegistrationQueue(addr)
	if !isNew1 {
		t.Fatal("expected isNew=true for first call")
	}

	q2, isNew2 := r.getOrCreateRegistrationQueue(addr)
	if isNew2 {
		t.Fatal("expected isNew=false for second call")
	}
	if q1 != q2 {
		t.Fatal("expected same queue instance on second call")
	}

	// Different address gets a new queue.
	addr2 := netip.MustParseAddrPort("192.168.1.2:5678")
	q3, isNew3 := r.getOrCreateRegistrationQueue(addr2)
	if !isNew3 {
		t.Fatal("expected isNew=true for different address")
	}
	if q3 == q1 {
		t.Fatal("expected different queue for different address")
	}
}
