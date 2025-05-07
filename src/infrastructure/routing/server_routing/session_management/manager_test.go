package session_management

import (
	"errors"
	"testing"
)

type fakeSession struct {
	internal, external []byte
}

func (f *fakeSession) InternalIP() []byte { return f.internal }
func (f *fakeSession) ExternalIP() []byte { return f.external }

func TestDefaultWorkerSessionManager(t *testing.T) {
	sm := NewDefaultWorkerSessionManager[*fakeSession]()

	t.Run("NotFoundBeforeAdd", func(t *testing.T) {
		if _, err := sm.GetByInternalIP([]byte{1, 2, 3, 4}); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("InvalidKeyLength", func(t *testing.T) {
		if _, err := sm.GetByInternalIP([]byte{1, 2, 3}); !errors.Is(err, ErrInvalidIPLength) {
			t.Errorf("expected ErrInvalidIPLength for internal, got %v", err)
		}
		if _, err := sm.GetByExternalIP([]byte{1, 2, 3, 4, 5}); !errors.Is(err, ErrInvalidIPLength) {
			t.Errorf("expected ErrInvalidIPLength for external, got %v", err)
		}
	})

	t.Run("AddAndGetInternal", func(t *testing.T) {
		s := &fakeSession{internal: []byte{1, 2, 3, 4}, external: []byte{5, 6, 7, 8}}
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
		s := &fakeSession{internal: []byte{9, 9, 9, 9}, external: []byte{10, 10, 10, 10}}
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
		s := &fakeSession{internal: []byte{11, 11, 11, 11}, external: []byte{12, 12, 12, 12}}
		sm.Add(s)
		sm.Delete(s)
		if _, err := sm.GetByInternalIP(s.internal); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("expected ErrSessionNotFound after delete, got %v", err)
		}
	})
}
