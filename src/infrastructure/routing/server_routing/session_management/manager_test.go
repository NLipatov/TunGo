package session_management

import (
	"errors"
	"net/netip"
	"testing"
)

type fakeSession struct {
	internal, external netip.Addr
}

func (f *fakeSession) InternalIP() netip.Addr { return f.internal }
func (f *fakeSession) ExternalIP() netip.Addr { return f.external }

func TestDefaultWorkerSessionManager(t *testing.T) {
	sm := NewDefaultWorkerSessionManager[*fakeSession]()

	t.Run("NotFoundBeforeAdd", func(t *testing.T) {
		addr, _ := netip.ParseAddr("1.2.3.4")
		if _, err := sm.GetByInternalIP(addr); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("AddAndGetInternal", func(t *testing.T) {
		internal, _ := netip.ParseAddr("1.2.3.4")
		external, _ := netip.ParseAddr("5.6.7.8")
		s := &fakeSession{internal: internal, external: external}
		sm.Add(s)
		got, err := sm.GetByInternalIP(s.internal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != s {
			t.Errorf("got %p, want %p", got, s)
		}
	})

	t.Run("AddAndGetExternal", func(t *testing.T) {
		internal, _ := netip.ParseAddr("9.9.9.9")
		external, _ := netip.ParseAddr("10.10.10.10")
		s := &fakeSession{internal: internal, external: external}
		sm.Add(s)
		got, err := sm.GetByExternalIP(s.external)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != s {
			t.Errorf("got %p, want %p", got, s)
		}
	})

	t.Run("DeleteRemoves", func(t *testing.T) {
		internal, _ := netip.ParseAddr("11.11.11.11")
		external, _ := netip.ParseAddr("12.12.12.12")
		s := &fakeSession{internal: internal, external: external}
		sm.Add(s)
		sm.Delete(s)
		if _, err := sm.GetByInternalIP(s.internal); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("expected ErrSessionNotFound after delete, got %v", err)
		}
	})
}
