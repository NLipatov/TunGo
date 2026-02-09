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

// fakeEgress is a no-op egress for testing Peer.Egress().
type fakeEgress struct{}

func (fakeEgress) SendDataIP([]byte) error  { return nil }
func (fakeEgress) SendControl([]byte) error { return nil }
func (fakeEgress) Close() error             { return nil }

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
