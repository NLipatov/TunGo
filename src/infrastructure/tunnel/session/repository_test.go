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
