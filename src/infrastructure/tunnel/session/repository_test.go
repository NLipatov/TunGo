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

func (f *fakeSession) InternalAddr() netip.Addr         { return f.internal }
func (f *fakeSession) ExternalAddrPort() netip.AddrPort { return f.external }
func (f *fakeSession) Crypto() connection.Crypto        { return nil }
func (f *fakeSession) RekeyController() rekey.FSM       { return nil }

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

func TestConcurrentRepository(t *testing.T) {
	inner := NewDefaultRepository()
	repo := NewConcurrentRepository(inner)

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
