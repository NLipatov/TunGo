package chacha20

import (
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func testSession(epoch Epoch) *DefaultUdpSession {
	key := make([]byte, chacha20poly1305.KeySize)
	key[0] = byte(epoch)
	aead, _ := chacha20poly1305.New(key)
	return NewUdpSessionWithCiphers([32]byte{}, aead, aead, false, epoch)
}

func TestEpochRing_NewWithInitialSession(t *testing.T) {
	s := testSession(0)
	r := NewEpochRing(4, 0, s)

	if r.Current() != 0 {
		t.Fatalf("expected Current()=0, got %d", r.Current())
	}
	if r.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", r.Len())
	}
	if r.Capacity() != 4 {
		t.Fatalf("expected Capacity()=4, got %d", r.Capacity())
	}
}

func TestEpochRing_NewWithNilSession(t *testing.T) {
	r := NewEpochRing(4, 0, nil)

	if r.Len() != 0 {
		t.Fatalf("expected Len()=0, got %d", r.Len())
	}
	if r.Current() != 0 {
		t.Fatalf("expected Current()=0 for empty ring, got %d", r.Current())
	}
	if _, ok := r.ResolveCurrent(); ok {
		t.Fatal("expected ResolveCurrent() to return false for empty ring")
	}
	if _, ok := r.Oldest(); ok {
		t.Fatal("expected Oldest() to return false for empty ring")
	}
}

func TestEpochRing_ResolveFindsInserted(t *testing.T) {
	r := NewEpochRing(4, 0, testSession(0))
	r.Insert(1, testSession(1))
	r.Insert(2, testSession(2))

	for _, e := range []Epoch{0, 1, 2} {
		s, ok := r.Resolve(e)
		if !ok {
			t.Fatalf("expected Resolve(%d) to succeed", e)
		}
		if s.Epoch() != e {
			t.Fatalf("expected Epoch()=%d, got %d", e, s.Epoch())
		}
	}

	if _, ok := r.Resolve(99); ok {
		t.Fatal("expected Resolve(99) to return false")
	}
}

func TestEpochRing_InsertEvictsOldestAtCapacity(t *testing.T) {
	r := NewEpochRing(3, 0, testSession(0))
	r.Insert(1, testSession(1))
	r.Insert(2, testSession(2))

	if r.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", r.Len())
	}

	// Insert 4th — should evict epoch 0.
	r.Insert(3, testSession(3))

	if r.Len() != 3 {
		t.Fatalf("expected Len()=3 after eviction, got %d", r.Len())
	}
	if _, ok := r.Resolve(0); ok {
		t.Fatal("expected epoch 0 to be evicted")
	}
	oldest, ok := r.Oldest()
	if !ok || oldest != 1 {
		t.Fatalf("expected Oldest()=1, got %d", oldest)
	}
}

func TestEpochRing_Current_ReturnsLastInserted(t *testing.T) {
	r := NewEpochRing(4, 0, testSession(0))
	r.Insert(5, testSession(5))
	r.Insert(10, testSession(10))

	if r.Current() != 10 {
		t.Fatalf("expected Current()=10, got %d", r.Current())
	}
}

func TestEpochRing_ResolveCurrent(t *testing.T) {
	r := NewEpochRing(4, 0, testSession(0))
	r.Insert(7, testSession(7))

	s, ok := r.ResolveCurrent()
	if !ok {
		t.Fatal("expected ResolveCurrent() to succeed")
	}
	if s.Epoch() != 7 {
		t.Fatalf("expected current session epoch=7, got %d", s.Epoch())
	}
}

func TestEpochRing_Remove(t *testing.T) {
	r := NewEpochRing(4, 0, testSession(0))
	r.Insert(1, testSession(1))
	r.Insert(2, testSession(2))

	if !r.Remove(1) {
		t.Fatal("expected Remove(1) to return true")
	}
	if r.Len() != 2 {
		t.Fatalf("expected Len()=2, got %d", r.Len())
	}
	if _, ok := r.Resolve(1); ok {
		t.Fatal("expected epoch 1 to be removed")
	}

	// Remove non-existent.
	if r.Remove(99) {
		t.Fatal("expected Remove(99) to return false")
	}
}

func TestEpochRing_Oldest(t *testing.T) {
	r := NewEpochRing(4, 5, testSession(5))
	r.Insert(10, testSession(10))

	oldest, ok := r.Oldest()
	if !ok || oldest != 5 {
		t.Fatalf("expected Oldest()=5, got %d ok=%v", oldest, ok)
	}
}

func TestEpochRing_ZeroizeAll(t *testing.T) {
	r := NewEpochRing(4, 0, testSession(0))
	r.Insert(1, testSession(1))
	r.Insert(2, testSession(2))

	r.ZeroizeAll()

	if r.Len() != 0 {
		t.Fatalf("expected ring to be empty after ZeroizeAll, got len=%d", r.Len())
	}
	if _, ok := r.ResolveCurrent(); ok {
		t.Fatal("expected no current session after ZeroizeAll")
	}
}
