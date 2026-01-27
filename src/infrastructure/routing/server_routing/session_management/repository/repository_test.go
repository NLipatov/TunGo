package repository

import (
	"errors"
	"net/netip"
	"testing"
	"tungo/application/network/connection"
	"tungo/application/network/rekey"
)

type fakeSession struct {
	internal netip.Addr
	external netip.AddrPort
}

func (f *fakeSession) InternalAddr() netip.Addr         { return f.internal }
func (f *fakeSession) ExternalAddrPort() netip.AddrPort { return f.external }
func (f *fakeSession) Transport() connection.Transport {
	return nil
}
func (f *fakeSession) Crypto() connection.Crypto {
	return nil
}
func (f *fakeSession) RekeyController() *rekey.Controller {
	return nil
}

func TestDefaultWorkerSessionManager(t *testing.T) {
	sm := NewDefaultWorkerSessionManager[*fakeSession]()

	t.Run("NotFoundBeforeAdd", func(t *testing.T) {
		addr, _ := netip.ParseAddr("1.2.3.4")
		if _, err := sm.GetByInternalAddrPort(addr); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("AddAndGetInternal", func(t *testing.T) {
		internal, _ := netip.ParseAddr("1.2.3.4")
		external, _ := netip.ParseAddrPort("5.6.7.8:9000")
		s := &fakeSession{internal: internal, external: external}
		sm.Add(s)
		got, err := sm.GetByInternalAddrPort(s.internal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != s {
			t.Errorf("got %p, want %p", got, s)
		}
	})

	t.Run("AddAndGetExternal", func(t *testing.T) {
		internal, _ := netip.ParseAddr("9.9.9.9")
		external, _ := netip.ParseAddrPort("10.10.10.10:9000")
		s := &fakeSession{internal: internal, external: external}
		sm.Add(s)
		got, err := sm.GetByExternalAddrPort(s.external)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != s {
			t.Errorf("got %p, want %p", got, s)
		}
	})

	t.Run("DeleteRemoves", func(t *testing.T) {
		internal, _ := netip.ParseAddr("11.11.11.11")
		external, _ := netip.ParseAddrPort("12.12.12.12:9000")
		s := &fakeSession{internal: internal, external: external}
		sm.Add(s)
		sm.Delete(s)
		if _, err := sm.GetByInternalAddrPort(s.internal); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("expected ErrSessionNotFound after delete, got %v", err)
		}
	})
}
