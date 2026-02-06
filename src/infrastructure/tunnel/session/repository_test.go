package session

import (
	"errors"
	"net/netip"
	"testing"

	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type fakeSession struct {
	internal netip.Addr
	external netip.AddrPort
}

func (f *fakeSession) InternalAddr() netip.Addr           { return f.internal }
func (f *fakeSession) ExternalAddrPort() netip.AddrPort   { return f.external }
func (f *fakeSession) Crypto() connection.Crypto          { return nil }
func (f *fakeSession) RekeyController() rekey.FSM         { return nil }
func (f *fakeSession) IsSourceAllowed(netip.Addr) bool { return true }

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
