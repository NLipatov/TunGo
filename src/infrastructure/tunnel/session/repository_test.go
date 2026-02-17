package session

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type fakeSession struct {
	internal netip.Addr
	external netip.AddrPort
}

func (f *fakeSession) InternalAddr() netip.Addr         { return f.internal }
func (f *fakeSession) ExternalAddrPort() netip.AddrPort { return f.external }
func (f *fakeSession) Crypto() connection.Crypto        { return nil }
func (f *fakeSession) RekeyController() rekey.FSM       { return nil }
func (f *fakeSession) IsSourceAllowed(netip.Addr) bool  { return true }

type fakeSessionWithCrypto struct {
	fakeSession
	crypto connection.Crypto
}

func (f *fakeSessionWithCrypto) Crypto() connection.Crypto { return f.crypto }

type fakeCryptoZeroizer struct {
	zeroized bool
}

func (f *fakeCryptoZeroizer) Encrypt(plaintext []byte) ([]byte, error)  { return plaintext, nil }
func (f *fakeCryptoZeroizer) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }
func (f *fakeCryptoZeroizer) Zeroize()                                  { f.zeroized = true }

type fakeCryptoNoZeroizer struct{}

func (f *fakeCryptoNoZeroizer) Encrypt(plaintext []byte) ([]byte, error)  { return plaintext, nil }
func (f *fakeCryptoNoZeroizer) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }

type fakeRouteCrypto struct {
	id uint64
}

func (f *fakeRouteCrypto) Encrypt(plaintext []byte) ([]byte, error)  { return plaintext, nil }
func (f *fakeRouteCrypto) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }
func (f *fakeRouteCrypto) RouteID() uint64                           { return f.id }

// fakeEgress is a no-op egress for testing Peer.Egress().
type fakeEgress struct{}

func (fakeEgress) SendDataIP([]byte) error  { return nil }
func (fakeEgress) SendControl([]byte) error { return nil }
func (fakeEgress) Close() error             { return nil }

type fakeAddrPortEgress struct {
	addrPort netip.AddrPort
	setCalls int
}

func (f *fakeAddrPortEgress) SendDataIP([]byte) error  { return nil }
func (f *fakeAddrPortEgress) SendControl([]byte) error { return nil }
func (f *fakeAddrPortEgress) Close() error             { return nil }
func (f *fakeAddrPortEgress) SetAddrPort(addr netip.AddrPort) {
	f.addrPort = addr
	f.setCalls++
}

func TestDefaultRepository(t *testing.T) {
	sm := NewDefaultRepository()

	t.Run("NotFoundBeforeAdd", func(t *testing.T) {
		addr, _ := netip.ParseAddr("1.2.3.4")
		if _, err := sm.GetByInternalAddrPort(addr); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("AddAndGetInternal", func(t *testing.T) {
		internal, _ := netip.ParseAddr("1.2.3.4")
		external, _ := netip.ParseAddrPort("5.6.7.8:9000")
		s := &fakeSession{internal: internal, external: external}
		p := NewPeer(s, nil)
		sm.Add(p)
		got, err := sm.GetByInternalAddrPort(s.internal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != p {
			t.Errorf("got %p, want %p", got, p)
		}
	})

	t.Run("AddAndGetExternal", func(t *testing.T) {
		internal, _ := netip.ParseAddr("9.9.9.9")
		external, _ := netip.ParseAddrPort("10.10.10.10:9000")
		s := &fakeSession{internal: internal, external: external}
		p := NewPeer(s, nil)
		sm.Add(p)
		got, err := sm.GetByExternalAddrPort(s.external)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != p {
			t.Errorf("got %p, want %p", got, p)
		}
	})

	t.Run("DeleteRemoves", func(t *testing.T) {
		internal, _ := netip.ParseAddr("11.11.11.11")
		external, _ := netip.ParseAddrPort("12.12.12.12:9000")
		s := &fakeSession{internal: internal, external: external}
		p := NewPeer(s, nil)
		sm.Add(p)
		sm.Delete(p)
		if _, err := sm.GetByInternalAddrPort(s.internal); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound after delete, got %v", err)
		}
	})
}

func TestDefaultRepository_GetByExternalAddrPort_NotFound(t *testing.T) {
	sm := NewDefaultRepository()
	addr := netip.MustParseAddrPort("99.99.99.99:1234")
	_, err := sm.GetByExternalAddrPort(addr)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPeer_Egress(t *testing.T) {
	eg := &fakeEgress{}
	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.2.3.4:5000"),
	}
	p := NewPeer(s, eg)
	if p.Egress() != eg {
		t.Fatal("expected Egress() to return the injected egress")
	}
}

func TestSession_RekeyController(t *testing.T) {
	rk := &fakeRekeyer{}
	fsm := rekey.NewStateMachine(rk, []byte("c2s"), []byte("s2c"), true)
	s := NewSession(nil, fsm, netip.MustParseAddr("10.0.0.1"), netip.MustParseAddrPort("1.2.3.4:5000"))
	if s.RekeyController() != fsm {
		t.Fatal("expected RekeyController() to return the injected FSM")
	}
}

// fakeRekeyer implements rekey.Rekeyer for session tests.
type fakeRekeyer struct{}

func (fakeRekeyer) Rekey(_, _ []byte) (uint16, error) { return 0, nil }
func (fakeRekeyer) SetSendEpoch(uint16)               {}
func (fakeRekeyer) RemoveEpoch(uint16) bool           { return true }

// fakeSessionWithIdentity implements SessionIdentity for testing TerminateByPubKey.
type fakeSessionWithIdentity struct {
	fakeSession
	pubKey []byte
}

func (f *fakeSessionWithIdentity) ClientPubKey() []byte { return f.pubKey }

func TestDefaultRepository_TerminateByPubKey(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)

	pubKey1 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	pubKey2 := []byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	// Create sessions with different public keys
	s1 := &fakeSessionWithIdentity{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.1"),
			external: netip.MustParseAddrPort("1.1.1.1:1000"),
		},
		pubKey: pubKey1,
	}
	s2 := &fakeSessionWithIdentity{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.2"),
			external: netip.MustParseAddrPort("2.2.2.2:2000"),
		},
		pubKey: pubKey2,
	}
	s3 := &fakeSessionWithIdentity{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.3"),
			external: netip.MustParseAddrPort("3.3.3.3:3000"),
		},
		pubKey: pubKey1, // Same pubkey as s1
	}

	p1 := NewPeer(s1, nil)
	p2 := NewPeer(s2, nil)
	p3 := NewPeer(s3, nil)

	repo.Add(p1)
	repo.Add(p2)
	repo.Add(p3)

	// Terminate all sessions for pubKey1 (should remove p1 and p3)
	count := repo.TerminateByPubKey(pubKey1)
	if count != 2 {
		t.Errorf("expected 2 sessions terminated, got %d", count)
	}

	// p1 and p3 should be gone
	if _, err := repo.GetByInternalAddrPort(s1.internal); err != ErrNotFound {
		t.Error("expected s1 to be removed")
	}
	if _, err := repo.GetByInternalAddrPort(s3.internal); err != ErrNotFound {
		t.Error("expected s3 to be removed")
	}

	// p2 should still exist
	if _, err := repo.GetByInternalAddrPort(s2.internal); err != nil {
		t.Error("expected s2 to still exist")
	}

	// Terminate with unknown pubkey should return 0
	count = repo.TerminateByPubKey([]byte{99, 99, 99})
	if count != 0 {
		t.Errorf("expected 0 sessions terminated for unknown key, got %d", count)
	}

	// Terminate with empty pubkey should return 0
	count = repo.TerminateByPubKey(nil)
	if count != 0 {
		t.Errorf("expected 0 sessions terminated for nil key, got %d", count)
	}
}

func TestDefaultRepository_TerminateByPubKey_ViaInterface(t *testing.T) {
	repo := NewDefaultRepository()

	pubKey := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	s := &fakeSessionWithIdentity{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.1"),
			external: netip.MustParseAddrPort("1.1.1.1:1000"),
		},
		pubKey: pubKey,
	}
	p := NewPeer(s, nil)
	repo.Add(p)

	// Test via RepositoryWithRevocation interface
	revocable, ok := repo.(RepositoryWithRevocation)
	if !ok {
		t.Fatal("DefaultRepository should implement RepositoryWithRevocation")
	}

	count := revocable.TerminateByPubKey(pubKey)
	if count != 1 {
		t.Errorf("expected 1 session terminated, got %d", count)
	}

	if _, err := repo.GetByInternalAddrPort(s.internal); err != ErrNotFound {
		t.Error("expected session to be removed")
	}
}

func TestDefaultRepository_BasicOperations(t *testing.T) {
	repo := NewDefaultRepository()

	internal := netip.MustParseAddr("10.0.0.1")
	external := netip.MustParseAddrPort("5.6.7.8:9000")
	s := &fakeSession{internal: internal, external: external}
	eg := &fakeEgress{}
	p := NewPeer(s, eg)

	repo.Add(p)

	got, err := repo.GetByInternalAddrPort(internal)
	if err != nil {
		t.Fatalf("GetByInternalAddrPort: unexpected error: %v", err)
	}
	if got != p {
		t.Fatal("GetByInternalAddrPort: wrong peer")
	}

	got, err = repo.GetByExternalAddrPort(external)
	if err != nil {
		t.Fatalf("GetByExternalAddrPort: unexpected error: %v", err)
	}
	if got != p {
		t.Fatal("GetByExternalAddrPort: wrong peer")
	}

	repo.Delete(p)
	_, err = repo.GetByInternalAddrPort(internal)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Delete, got %v", err)
	}
}

func TestDefaultRepository_FindByDestinationIP(t *testing.T) {
	repo := NewDefaultRepository()

	fastInternal := netip.MustParseAddr("10.0.0.10")
	fastExternal := netip.MustParseAddrPort("1.1.1.1:1000")
	fastPeer := NewPeer(NewSession(nil, nil, fastInternal, fastExternal), nil)
	repo.Add(fastPeer)

	allowedInternal := netip.MustParseAddr("10.0.0.20")
	allowedExternal := netip.MustParseAddrPort("2.2.2.2:2000")
	allowedPeer := NewPeer(NewSessionWithAuth(
		nil, nil, allowedInternal, allowedExternal, nil,
		[]netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")},
	), nil)
	repo.Add(allowedPeer)

	t.Run("FastPathInternalExactMatch", func(t *testing.T) {
		got, err := repo.FindByDestinationIP(netip.MustParseAddr("::ffff:10.0.0.10"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != fastPeer {
			t.Fatalf("got %p, want %p", got, fastPeer)
		}
	})

	ipv6Internal := netip.MustParseAddr("10.0.0.30")
	ipv6External := netip.MustParseAddrPort("3.3.3.3:3000")
	ipv6Peer := NewPeer(NewSessionWithAuth(
		nil, nil, ipv6Internal, ipv6External, nil,
		[]netip.Prefix{netip.MustParsePrefix("fd00::2/128")},
	), nil)
	repo.Add(ipv6Peer)

	t.Run("FastPathAllowedIPv6HostRoute", func(t *testing.T) {
		got, err := repo.FindByDestinationIP(netip.MustParseAddr("fd00::2"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != ipv6Peer {
			t.Fatalf("got %p, want %p", got, ipv6Peer)
		}
	})

	t.Run("SlowPathAllowedIPsMatch", func(t *testing.T) {
		got, err := repo.FindByDestinationIP(netip.MustParseAddr("192.168.50.42"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != allowedPeer {
			t.Fatalf("got %p, want %p", got, allowedPeer)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := repo.FindByDestinationIP(netip.MustParseAddr("203.0.113.77"))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestDefaultRepository_Delete_ZeroizesCrypto(t *testing.T) {
	repo := NewDefaultRepository()
	crypto := &fakeCryptoZeroizer{}

	s := &fakeSessionWithCrypto{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.1.0.1"),
			external: netip.MustParseAddrPort("9.9.9.9:9999"),
		},
		crypto: crypto,
	}
	p := NewPeer(s, nil)
	repo.Add(p)

	repo.Delete(p)

	if !p.IsClosed() {
		t.Fatal("expected peer to be marked closed")
	}
	if !crypto.zeroized {
		t.Fatal("expected crypto to be zeroized")
	}
}

func TestDefaultRepository_Delete_NonZeroizerCrypto(t *testing.T) {
	repo := NewDefaultRepository()
	crypto := &fakeCryptoNoZeroizer{}

	s := &fakeSessionWithCrypto{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.1.0.2"),
			external: netip.MustParseAddrPort("9.9.9.8:9998"),
		},
		crypto: crypto,
	}
	p := NewPeer(s, nil)
	repo.Add(p)
	repo.Delete(p)

	if _, err := repo.GetByInternalAddrPort(s.internal); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Delete, got %v", err)
	}
}

// --- Peer activity tracking tests ---

func TestPeer_NewPeer_InitializesLastActivity(t *testing.T) {
	before := time.Now()
	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.2.3.4:5000"),
	}
	p := NewPeer(s, nil)
	after := time.Now()

	la := p.LastActivity()
	if la.Before(before.Truncate(time.Second)) || la.After(after.Add(time.Second)) {
		t.Fatalf("lastActivity %v not within [%v, %v]", la, before, after)
	}
}

func TestPeer_TouchActivity_UpdatesTime(t *testing.T) {
	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.2.3.4:5000"),
	}
	p := NewPeer(s, nil)

	// Force lastActivity to the past
	p.lastActivity.Store(time.Now().Add(-10 * time.Minute).Unix())
	old := p.LastActivity()

	p.TouchActivity()
	updated := p.LastActivity()

	if !updated.After(old) {
		t.Fatalf("expected TouchActivity to advance time: old=%v updated=%v", old, updated)
	}
}

func TestPeer_LastActivity_ReturnsStoredTime(t *testing.T) {
	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.2.3.4:5000"),
	}
	p := NewPeer(s, nil)

	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	p.lastActivity.Store(fixed.Unix())

	got := p.LastActivity()
	if got.Unix() != fixed.Unix() {
		t.Fatalf("expected %v, got %v", fixed, got)
	}
}

// --- ReapIdle tests ---

func TestDefaultRepository_ReapIdle_RemovesIdlePeers(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)

	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.1.1.1:1000"),
	}
	p := NewPeer(s, nil)
	// Set activity far in the past
	p.lastActivity.Store(time.Now().Add(-5 * time.Minute).Unix())
	repo.Add(p)

	count := repo.ReapIdle(30 * time.Second)
	if count != 1 {
		t.Fatalf("expected 1 reaped, got %d", count)
	}
	if !p.IsClosed() {
		t.Fatal("expected peer to be marked closed")
	}
	if _, err := repo.GetByInternalAddrPort(s.internal); !errors.Is(err, ErrNotFound) {
		t.Fatal("expected peer removed from repo")
	}
}

func TestDefaultRepository_ReapIdle_KeepsActivePeers(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)

	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.2"),
		external: netip.MustParseAddrPort("2.2.2.2:2000"),
	}
	p := NewPeer(s, nil)
	// Activity is fresh (just created)
	repo.Add(p)

	count := repo.ReapIdle(30 * time.Second)
	if count != 0 {
		t.Fatalf("expected 0 reaped, got %d", count)
	}
	if _, err := repo.GetByInternalAddrPort(s.internal); err != nil {
		t.Fatal("expected peer to still exist")
	}
}

func TestDefaultRepository_ReapIdle_MixedActiveAndIdle(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)

	idle := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.1.1.1:1000"),
	}
	active := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.2"),
		external: netip.MustParseAddrPort("2.2.2.2:2000"),
	}

	pIdle := NewPeer(idle, nil)
	pIdle.lastActivity.Store(time.Now().Add(-2 * time.Minute).Unix())
	repo.Add(pIdle)

	pActive := NewPeer(active, nil)
	// pActive has fresh lastActivity from NewPeer
	repo.Add(pActive)

	count := repo.ReapIdle(30 * time.Second)
	if count != 1 {
		t.Fatalf("expected 1 reaped, got %d", count)
	}

	// Idle peer removed
	if _, err := repo.GetByInternalAddrPort(idle.internal); !errors.Is(err, ErrNotFound) {
		t.Fatal("expected idle peer removed")
	}
	// Active peer survives
	if _, err := repo.GetByInternalAddrPort(active.internal); err != nil {
		t.Fatal("expected active peer to survive")
	}
}

func TestDefaultRepository_ReapIdle_EmptyRepo(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)
	count := repo.ReapIdle(30 * time.Second)
	if count != 0 {
		t.Fatalf("expected 0 reaped on empty repo, got %d", count)
	}
}

func TestDefaultRepository_ReapIdle_ViaInterface(t *testing.T) {
	repo := NewDefaultRepository()

	reaper, ok := repo.(IdleReaper)
	if !ok {
		t.Fatal("DefaultRepository should implement IdleReaper")
	}

	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.1.1.1:1000"),
	}
	p := NewPeer(s, nil)
	p.lastActivity.Store(time.Now().Add(-5 * time.Minute).Unix())
	repo.Add(p)

	count := reaper.ReapIdle(30 * time.Second)
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
}

func TestDefaultRepository_ReapIdle_ZeroizesCrypto(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)
	crypto := &fakeCryptoZeroizer{}

	s := &fakeSessionWithCrypto{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.1"),
			external: netip.MustParseAddrPort("1.1.1.1:1000"),
		},
		crypto: crypto,
	}
	p := NewPeer(s, nil)
	p.lastActivity.Store(time.Now().Add(-5 * time.Minute).Unix())
	repo.Add(p)

	repo.ReapIdle(30 * time.Second)

	if !crypto.zeroized {
		t.Fatal("expected crypto to be zeroized on reap")
	}
}

func TestDefaultRepository_Delete_CleansUpAllowedAddrs(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)

	internal := netip.MustParseAddr("10.0.0.1")
	external := netip.MustParseAddrPort("1.1.1.1:1000")
	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.1/32"),
		netip.MustParsePrefix("fd00::2/128"),
	}

	s := NewSessionWithAuth(nil, nil, internal, external, nil, allowedIPs)
	p := NewPeer(s, nil)
	repo.Add(p)

	// Verify allowed addrs are indexed
	if _, found := repo.allowedAddrToPeer[netip.MustParseAddr("192.168.1.1")]; !found {
		t.Fatal("expected 192.168.1.1 in allowedAddrToPeer after Add")
	}
	if _, found := repo.allowedAddrToPeer[netip.MustParseAddr("fd00::2")]; !found {
		t.Fatal("expected fd00::2 in allowedAddrToPeer after Add")
	}

	// Delete should clean up the allowed addr index
	repo.Delete(p)

	if _, found := repo.allowedAddrToPeer[netip.MustParseAddr("192.168.1.1")]; found {
		t.Fatal("expected 192.168.1.1 removed from allowedAddrToPeer after Delete")
	}
	if _, found := repo.allowedAddrToPeer[netip.MustParseAddr("fd00::2")]; found {
		t.Fatal("expected fd00::2 removed from allowedAddrToPeer after Delete")
	}
}

func TestPeer_SetLastActivityForTest(t *testing.T) {
	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.2.3.4:5000"),
	}
	p := NewPeer(s, nil)

	fixed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	p.SetLastActivityForTest(fixed)
	if got := p.LastActivity().Unix(); got != fixed {
		t.Fatalf("SetLastActivityForTest: got %d, want %d", got, fixed)
	}
}

func TestPeer_MarkClosedForTest(t *testing.T) {
	s := &fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.2.3.4:5000"),
	}
	p := NewPeer(s, nil)
	if p.IsClosed() {
		t.Fatal("new peer should not be closed")
	}
	p.MarkClosedForTest()
	if !p.IsClosed() {
		t.Fatal("expected peer to be closed after MarkClosedForTest")
	}
}

func TestDefaultRepository_ReapIdle_MultipleIdlePeers(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)

	for i := 1; i <= 5; i++ {
		s := &fakeSession{
			internal: netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}),
			external: netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 1, 1, byte(i)}), uint16(1000+i)),
		}
		p := NewPeer(s, nil)
		p.lastActivity.Store(time.Now().Add(-time.Hour).Unix())
		repo.Add(p)
	}

	count := repo.ReapIdle(30 * time.Second)
	if count != 5 {
		t.Fatalf("expected 5 reaped, got %d", count)
	}
}

func TestDefaultRepository_AllPeers_ReturnsSnapshot(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)

	p1 := NewPeer(&fakeSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.1.1.1:1001"),
	}, nil)
	p2 := NewPeer(&fakeSession{
		internal: netip.MustParseAddr("10.0.0.2"),
		external: netip.MustParseAddrPort("1.1.1.2:1002"),
	}, nil)
	repo.Add(p1)
	repo.Add(p2)

	peers := repo.AllPeers()
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}

	var found1, found2 bool
	for _, p := range peers {
		if p == p1 {
			found1 = true
		}
		if p == p2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Fatal("expected snapshot to contain both peers")
	}

	peers[0] = nil
	again := repo.AllPeers()
	if len(again) != 2 {
		t.Fatalf("expected 2 peers in fresh snapshot, got %d", len(again))
	}
}

func TestDefaultRepository_UpdateExternalAddr_ReindexesAndUpdatesEgress(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)
	oldAddr := netip.MustParseAddrPort("198.51.100.10:5000")
	newAddr := netip.MustParseAddrPort("198.51.100.11:6000")
	egress := &fakeAddrPortEgress{}

	p := NewPeer(&fakeSession{
		internal: netip.MustParseAddr("10.9.0.1"),
		external: oldAddr,
	}, egress)
	repo.Add(p)

	repo.UpdateExternalAddr(p, newAddr)

	if _, err := repo.GetByExternalAddrPort(oldAddr); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected old address to be removed, got %v", err)
	}
	got, err := repo.GetByExternalAddrPort(newAddr)
	if err != nil {
		t.Fatalf("expected new address lookup to succeed, got %v", err)
	}
	if got != p {
		t.Fatal("expected repository to keep same peer instance after reindex")
	}
	if p.ExternalAddrPort() != newAddr {
		t.Fatalf("expected peer external address to be updated to %v, got %v", newAddr, p.ExternalAddrPort())
	}
	if egress.setCalls != 1 || egress.addrPort != newAddr {
		t.Fatalf("expected egress SetAddrPort to be called once with %v, got calls=%d addr=%v",
			newAddr, egress.setCalls, egress.addrPort)
	}
}

func TestDefaultRepository_UpdateExternalAddr_ClosedPeerNoOp(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)
	oldAddr := netip.MustParseAddrPort("203.0.113.10:5000")
	newAddr := netip.MustParseAddrPort("203.0.113.11:6000")
	egress := &fakeAddrPortEgress{}

	p := NewPeer(&fakeSession{
		internal: netip.MustParseAddr("10.9.0.2"),
		external: oldAddr,
	}, egress)
	repo.Add(p)
	p.MarkClosedForTest()

	repo.UpdateExternalAddr(p, newAddr)

	if _, err := repo.GetByExternalAddrPort(oldAddr); err != nil {
		t.Fatalf("expected old address to stay indexed for closed peer, got %v", err)
	}
	if _, err := repo.GetByExternalAddrPort(newAddr); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected new address not to be indexed for closed peer, got %v", err)
	}
	if p.ExternalAddrPort() != oldAddr {
		t.Fatalf("expected peer external address to remain %v, got %v", oldAddr, p.ExternalAddrPort())
	}
	if egress.setCalls != 0 {
		t.Fatalf("expected no egress address update for closed peer, got %d calls", egress.setCalls)
	}
}

func TestPeer_CryptoRLock_SuccessAndUnlock(t *testing.T) {
	p := NewPeer(&fakeSession{
		internal: netip.MustParseAddr("10.0.1.1"),
		external: netip.MustParseAddrPort("192.0.2.10:1000"),
	}, nil)

	if !p.CryptoRLock() {
		t.Fatal("expected lock acquisition for active peer")
	}
	p.CryptoRUnlock()
}

func TestPeer_CryptoRLock_ReturnsFalseWhenClosed(t *testing.T) {
	p := NewPeer(&fakeSession{
		internal: netip.MustParseAddr("10.0.1.2"),
		external: netip.MustParseAddrPort("192.0.2.11:1001"),
	}, nil)
	p.MarkClosedForTest()

	if p.CryptoRLock() {
		t.Fatal("expected lock acquisition to fail for closed peer")
	}
}

func TestDefaultRepository_GetByRouteID(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)
	routeID := uint64(0x1122334455667788)

	sess := &fakeSessionWithCrypto{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.10.0.1"),
			external: netip.MustParseAddrPort("203.0.113.20:5000"),
		},
		crypto: &fakeRouteCrypto{id: routeID},
	}
	peer := NewPeer(sess, nil)
	repo.Add(peer)

	got, err := repo.GetByRouteID(routeID)
	if err != nil {
		t.Fatalf("expected route-id lookup success, got %v", err)
	}
	if got != peer {
		t.Fatalf("expected peer %p, got %p", peer, got)
	}
}

func TestDefaultRepository_GetByRouteID_NotFoundAfterDelete(t *testing.T) {
	repo := NewDefaultRepository().(*DefaultRepository)
	routeID := uint64(0xaabbccddeeff0011)

	sess := &fakeSessionWithCrypto{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.10.0.2"),
			external: netip.MustParseAddrPort("203.0.113.21:5001"),
		},
		crypto: &fakeRouteCrypto{id: routeID},
	}
	peer := NewPeer(sess, nil)
	repo.Add(peer)
	repo.Delete(peer)

	if _, err := repo.GetByRouteID(routeID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
