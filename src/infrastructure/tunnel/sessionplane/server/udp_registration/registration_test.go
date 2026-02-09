package udp_registration

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/session"
)

// udpRegLogger is a no-op logger.
type udpRegLogger struct{}

func (udpRegLogger) Printf(string, ...any) {}

// udpRegRekeyer is a mock rekeyer.
type udpRegRekeyer struct{}

func (udpRegRekeyer) Rekey(_, _ []byte) (uint16, error) { return 0, nil }
func (udpRegRekeyer) SetSendEpoch(uint16)               {}
func (udpRegRekeyer) RemoveEpoch(uint16) bool           { return true }

// udpRegCrypto is a mock crypto.
type udpRegCrypto struct{}

func (udpRegCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (udpRegCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

// udpRegHandshake is a mock handshake that reads from registration queue.
type udpRegHandshake struct {
	clientIndex int
	err         error
	id          [32]byte
	c2s, s2c    []byte
}

func (h *udpRegHandshake) Id() [32]byte              { return h.id }
func (h *udpRegHandshake) KeyClientToServer() []byte { return h.c2s }
func (h *udpRegHandshake) KeyServerToClient() []byte { return h.s2c }
func (h *udpRegHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	return nil
}
func (h *udpRegHandshake) ServerSideHandshake(transport connection.Transport) (int, error) {
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
	return h.clientIndex, nil
}

type udpRegHandshakeFactory struct {
	handshake *udpRegHandshake
}

func (f *udpRegHandshakeFactory) NewHandshake() connection.Handshake {
	return f.handshake
}

type udpRegCryptoFactory struct {
	crypto connection.Crypto
	ctrl   *rekey.StateMachine
	err    error
}

func (f *udpRegCryptoFactory) FromHandshake(connection.Handshake, bool) (connection.Crypto, *rekey.StateMachine, error) {
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

func TestNewRegistrar_CreatesEmptyRegistrations(t *testing.T) {
	ctx := context.Background()
	r := NewRegistrar(ctx, nil, nil, udpRegLogger{}, nil, nil, netip.MustParsePrefix("10.0.0.0/24"))
	if r == nil {
		t.Fatal("expected non-nil registrar")
	}
	if len(r.Registrations()) != 0 {
		t.Fatal("expected empty registrations map")
	}
}

func TestGetOrCreateRegistrationQueue_CreatesNew(t *testing.T) {
	ctx := context.Background()
	r := NewRegistrar(ctx, nil, nil, udpRegLogger{}, nil, nil, netip.MustParsePrefix("10.0.0.0/24"))

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, isNew := r.GetOrCreateRegistrationQueue(addr)
	if !isNew {
		t.Fatal("expected isNew=true for first call")
	}
	if q == nil {
		t.Fatal("expected non-nil queue")
	}

	// Second call returns existing.
	q2, isNew2 := r.GetOrCreateRegistrationQueue(addr)
	if isNew2 {
		t.Fatal("expected isNew=false for second call")
	}
	if q2 != q {
		t.Fatal("expected same queue on second call")
	}
}

func TestCloseAll_ClearsRegistrations(t *testing.T) {
	ctx := context.Background()
	r := NewRegistrar(ctx, nil, nil, udpRegLogger{}, nil, nil, netip.MustParsePrefix("10.0.0.0/24"))

	r.GetOrCreateRegistrationQueue(netip.MustParseAddrPort("192.168.1.1:1234"))
	r.GetOrCreateRegistrationQueue(netip.MustParseAddrPort("192.168.1.2:5678"))

	if len(r.Registrations()) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(r.Registrations()))
	}

	r.CloseAll()

	if len(r.Registrations()) != 0 {
		t.Fatalf("expected 0 registrations after CloseAll, got %d", len(r.Registrations()))
	}
}

func TestEnqueuePacket_CreatesQueueAndStartsRegistration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewDefaultRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			err: errors.New("handshake fail"), // cause registration to fail
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}

	r := NewRegistrar(ctx, listener, repo, udpRegLogger{}, hf, cf, netip.MustParsePrefix("10.0.0.0/24"))

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	r.EnqueuePacket(addr, []byte("hello"))

	// After registration failure, the queue should be removed.
	// Give time for the registration goroutine to complete.
	time.Sleep(100 * time.Millisecond)
	if len(r.Registrations()) != 0 {
		t.Fatalf("expected registration removed after failure, got %d", len(r.Registrations()))
	}
}

func TestRegisterClient_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewDefaultRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			clientIndex: 1,
			c2s:         make([]byte, 32),
			s2c:         make([]byte, 32),
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}

	r := NewRegistrar(ctx, listener, repo, udpRegLogger{}, hf, cf, netip.MustParsePrefix("10.0.0.0/24"))

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.GetOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.RegisterClient(addr, q)
		close(done)
	}()

	// Feed the handshake a packet (simulate client hello).
	q.Enqueue([]byte("client-hello"))

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
	repo := session.NewDefaultRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			clientIndex: 1,
			c2s:         make([]byte, 32),
			s2c:         make([]byte, 32),
		},
	}
	cf := &udpRegCryptoFactory{err: errors.New("crypto failed")}

	r := NewRegistrar(ctx, listener, repo, udpRegLogger{}, hf, cf, netip.MustParsePrefix("10.0.0.0/24"))

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.GetOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.RegisterClient(addr, q)
		close(done)
	}()

	q.Enqueue([]byte("client-hello"))

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

func TestRegisterClient_NegativeClientIndex_FailsAllocation(t *testing.T) {
	// Negative clientIndex causes AllocateClientIP to fail.
	// UDP registrar logs and returns silently, no session added.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &udpRegListener{}
	repo := session.NewDefaultRepository()

	hf := &udpRegHandshakeFactory{
		handshake: &udpRegHandshake{
			clientIndex: -1, // invalid
			c2s:         make([]byte, 32),
			s2c:         make([]byte, 32),
		},
	}
	cf := &udpRegCryptoFactory{
		crypto: udpRegCrypto{},
		ctrl:   rekey.NewStateMachine(udpRegRekeyer{}, []byte("c2s"), []byte("s2c"), true),
	}

	r := NewRegistrar(ctx, listener, repo, udpRegLogger{}, hf, cf, netip.MustParsePrefix("10.0.0.0/24"))

	addr := netip.MustParseAddrPort("192.168.1.1:1234")
	q, _ := r.GetOrCreateRegistrationQueue(addr)

	done := make(chan struct{})
	go func() {
		r.RegisterClient(addr, q)
		close(done)
	}()

	q.Enqueue([]byte("client-hello"))

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

func TestGetOrCreateRegistrationQueue_SecondCallReusesQueue(t *testing.T) {
	ctx := context.Background()
	r := NewRegistrar(ctx, nil, nil, udpRegLogger{}, nil, nil, netip.MustParsePrefix("10.0.0.0/24"))

	addr := netip.MustParseAddrPort("192.168.1.1:1234")

	q1, isNew1 := r.GetOrCreateRegistrationQueue(addr)
	if !isNew1 {
		t.Fatal("expected isNew=true for first call")
	}

	q2, isNew2 := r.GetOrCreateRegistrationQueue(addr)
	if isNew2 {
		t.Fatal("expected isNew=false for second call")
	}
	if q1 != q2 {
		t.Fatal("expected same queue instance on second call")
	}

	// Different address gets a new queue.
	addr2 := netip.MustParseAddrPort("192.168.1.2:5678")
	q3, isNew3 := r.GetOrCreateRegistrationQueue(addr2)
	if !isNew3 {
		t.Fatal("expected isNew=true for different address")
	}
	if q3 == q1 {
		t.Fatal("expected different queue for different address")
	}
}
