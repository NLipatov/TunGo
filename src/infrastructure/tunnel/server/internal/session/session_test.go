package session

import (
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"tungo/infrastructure/tunnel/internal/rekey"
)

type testCrypto struct {
	mu        sync.Mutex
	zeroized  bool
	decryptCh chan struct{}
	releaseCh chan struct{}
}

func (c *testCrypto) Encrypt(data []byte) ([]byte, error) { return data, nil }
func (c *testCrypto) Decrypt(data []byte) ([]byte, error) {
	if c.decryptCh != nil {
		close(c.decryptCh)
		<-c.releaseCh
	}
	return data, nil
}
func (c *testCrypto) Zeroize() {
	c.mu.Lock()
	c.zeroized = true
	c.mu.Unlock()
}

type routeCrypto struct {
	testCrypto
	id uint64
}

func (c *routeCrypto) RouteID() uint64 { return c.id }

type testEgress struct {
	closed bool
	addr   netip.AddrPort
}

func (*testEgress) Send([]byte) error { return nil }
func (e *testEgress) Close() error {
	e.closed = true
	return nil
}
func (e *testEgress) SetAddrPort(addr netip.AddrPort) { e.addr = addr }

type blockingEgress struct {
	started chan struct{}
	release chan struct{}
}

func (e *blockingEgress) Send([]byte) error {
	close(e.started)
	<-e.release
	return nil
}
func (*blockingEgress) Close() error { return nil }

func TestPeer_AuthAndLifecycle(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.2")
	external := netip.MustParseAddrPort("192.0.2.1:1000")
	allowed := netip.MustParsePrefix("10.1.0.0/16")
	crypto := &testCrypto{}
	peer := newPeerWithAuth(
		crypto, nil, internal, external, []byte{1, 2, 3}, []netip.Prefix{allowed}, &testEgress{},
	)

	if peer.InternalAddr() != internal || peer.ExternalAddrPort() != external {
		t.Fatal("peer addresses were not preserved")
	}
	if !peer.IsSourceAllowed(internal) || !peer.IsSourceAllowed(netip.MustParseAddr("10.1.2.3")) {
		t.Fatal("expected internal and configured source addresses to be allowed")
	}
	if peer.IsSourceAllowed(netip.MustParseAddr("10.2.0.1")) {
		t.Fatal("unexpected source address allowed")
	}
	plaintext, err := peer.Decrypt([]byte("data"))
	if err != nil || string(plaintext) != "data" {
		t.Fatalf("decrypt: plaintext=%q err=%v", plaintext, err)
	}
	if err := peer.Send([]byte("data")); err != nil {
		t.Fatalf("send: %v", err)
	}
	peer.markClosed()
	if _, err := peer.Decrypt(nil); !errors.Is(err, ErrPeerClosed) {
		t.Fatalf("closed peer decrypt error = %v", err)
	}
	if err := peer.Send(nil); !errors.Is(err, ErrPeerClosed) {
		t.Fatalf("closed peer send error = %v", err)
	}
}

func TestRepository_AddRouteUpdateAndDelete(t *testing.T) {
	repo := NewRepository()
	internal := netip.MustParseAddr("10.0.0.2")
	external := netip.MustParseAddrPort("192.0.2.1:1000")
	updated := netip.MustParseAddrPort("192.0.2.2:2000")
	crypto := &routeCrypto{id: 42}
	egress := &testEgress{}
	peer := newPeerWithAuth(
		crypto, nil, internal, external, []byte("identity"),
		[]netip.Prefix{netip.MustParsePrefix("10.10.0.0/16"), netip.MustParsePrefix("2001:db8::2/128")},
		egress,
	)
	repo.Add(peer)

	assertPeer := func(got *Peer, err error) {
		t.Helper()
		if err != nil || got != peer {
			t.Fatalf("lookup: peer=%p err=%v", got, err)
		}
	}
	got, err := repo.GetByInternalAddrPort(internal)
	assertPeer(got, err)
	got, err = repo.GetByRouteID(42)
	assertPeer(got, err)
	got, err = repo.FindByDestinationIP(netip.MustParseAddr("10.10.3.4"))
	assertPeer(got, err)
	got, err = repo.FindByDestinationIP(netip.MustParseAddr("2001:db8::2"))
	assertPeer(got, err)

	repo.UpdateExternalAddr(peer, updated)
	if peer.ExternalAddrPort() != updated {
		t.Fatalf("peer external address = %v, want %v", peer.ExternalAddrPort(), updated)
	}
	if egress.addr != updated {
		t.Fatalf("egress address = %v, want %v", egress.addr, updated)
	}

	repo.Delete(peer)
	if !egress.closed || !crypto.zeroized || !peer.IsClosed() {
		t.Fatalf("delete did not close and zeroize peer: egress=%v crypto=%v peer=%v", egress.closed, crypto.zeroized, peer.IsClosed())
	}
	if _, err := repo.GetByInternalAddrPort(internal); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted peer lookup error = %v", err)
	}
	if _, err := repo.GetByRouteID(42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted route ID lookup error = %v", err)
	}
}

func TestRepository_DeleteWaitsForDecrypt(t *testing.T) {
	crypto := &testCrypto{decryptCh: make(chan struct{}), releaseCh: make(chan struct{})}
	peer := NewPeer(crypto, nil, netip.MustParseAddr("10.0.0.2"), netip.MustParseAddrPort("192.0.2.1:1"), nil)
	repo := NewRepository()
	repo.Add(peer)

	decryptDone := make(chan struct{})
	go func() {
		_, _ = peer.Decrypt([]byte("x"))
		close(decryptDone)
	}()
	<-crypto.decryptCh
	deleteDone := make(chan struct{})
	go func() {
		repo.Delete(peer)
		close(deleteDone)
	}()
	select {
	case <-deleteDone:
		t.Fatal("delete completed while decrypt still held crypto")
	case <-time.After(20 * time.Millisecond):
	}
	close(crypto.releaseCh)
	<-decryptDone
	<-deleteDone
	if !crypto.zeroized {
		t.Fatal("crypto was not zeroized")
	}
}

func TestRepository_DeleteWaitsForSend(t *testing.T) {
	crypto := &testCrypto{}
	egress := &blockingEgress{started: make(chan struct{}), release: make(chan struct{})}
	peer := newPeer(
		crypto, nil, netip.MustParseAddr("10.0.0.2"), netip.MustParseAddrPort("192.0.2.1:1"), egress,
	)
	repo := NewRepository()
	repo.Add(peer)

	sendDone := make(chan struct{})
	go func() {
		_ = peer.Send([]byte("x"))
		close(sendDone)
	}()
	<-egress.started
	deleteDone := make(chan struct{})
	go func() {
		repo.Delete(peer)
		close(deleteDone)
	}()
	select {
	case <-deleteDone:
		t.Fatal("delete completed while send still held crypto")
	case <-time.After(20 * time.Millisecond):
	}
	close(egress.release)
	<-sendDone
	<-deleteDone
	if !crypto.zeroized {
		t.Fatal("crypto was not zeroized")
	}
}

func TestRepository_RevocationAndIdleReaping(t *testing.T) {
	repo := NewRepository()
	pubKey := []byte("client-key")
	active := NewPeerWithAuth(nil, nil, netip.MustParseAddr("10.0.0.2"), netip.MustParseAddrPort("192.0.2.1:1"), pubKey, nil, nil)
	idle := NewPeerWithAuth(nil, nil, netip.MustParseAddr("10.0.0.3"), netip.MustParseAddrPort("192.0.2.2:2"), []byte("other"), nil, nil)
	idle.lastActivity.Store(time.Now().Add(-time.Hour).Unix())
	repo.Add(active)
	repo.Add(idle)

	if count := repo.ReapIdle(time.Minute); count != 1 || !idle.IsClosed() {
		t.Fatalf("reaped=%d idleClosed=%v", count, idle.IsClosed())
	}
	if count := repo.TerminateByPubKey(pubKey); count != 1 || !active.IsClosed() {
		t.Fatalf("revoked=%d activeClosed=%v", count, active.IsClosed())
	}
}

func TestPeer_RekeyCoordinatorIsConcrete(t *testing.T) {
	coordinator := rekey.NewServerRekeyCoordinator(nil)
	peer := NewPeer(nil, coordinator, netip.Addr{}, netip.AddrPort{}, nil)
	if peer.RekeyController() != coordinator {
		t.Fatal("peer did not preserve rekey coordinator")
	}
}
