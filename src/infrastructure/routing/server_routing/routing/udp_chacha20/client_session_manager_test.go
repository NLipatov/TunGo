package udp_chacha20

import (
	"bytes"
	"errors"
	"testing"
)

func TestWorkerSessionManager(t *testing.T) {
	t.Run("NotFoundBeforeAdd", func(t *testing.T) {
		sm := NewUdpWorkerSessionManager()
		if _, err := sm.GetByInternalIP([]byte{1, 2, 3, 4}); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("InvalidKeyLength", func(t *testing.T) {
		sm := NewUdpWorkerSessionManager()
		if _, err := sm.GetByInternalIP([]byte{1, 2, 3}); !errors.Is(err, ErrInvalidIPLength) {
			t.Errorf("expected ErrInvalidIPLength, got %v", err)
		}
		if _, err := sm.GetByExternalIP([]byte{1, 2, 3, 4, 5}); !errors.Is(err, ErrInvalidIPLength) {
			t.Errorf("expected ErrInvalidIPLength, got %v", err)
		}
	})

	t.Run("AddAndGet", func(t *testing.T) {
		sm := NewUdpWorkerSessionManager()
		s := session{internalIP: []byte{1, 2, 3, 4}, externalIP: []byte{5, 6, 7, 8}}
		sm.Add(s)
		got, err := sm.GetByInternalIP(s.internalIP)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got.internalIP, s.internalIP) {
			t.Errorf("got %v, want %v", got.internalIP, s.internalIP)
		}
	})

	t.Run("DeleteRemoves", func(t *testing.T) {
		sm := NewUdpWorkerSessionManager()
		s := session{internalIP: []byte{9, 9, 9, 9}, externalIP: []byte{10, 10, 10, 10}}
		sm.Add(s)
		sm.Delete(s)
		if _, err := sm.GetByInternalIP(s.internalIP); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("expected ErrSessionNotFound after delete, got %v", err)
		}
	})
}
