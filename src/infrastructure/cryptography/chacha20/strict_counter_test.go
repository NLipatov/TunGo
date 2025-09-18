package chacha20

import (
	"encoding/binary"
	"errors"
	"testing"
)

func makeNonce(high uint32, low uint64) [12]byte {
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[0:8], low)
	binary.BigEndian.PutUint32(nonce[8:12], high)
	return nonce
}

func TestStrictCounterValidate(t *testing.T) {
	sc := NewStrictCounter()

	// First nonce should be accepted
	if err := sc.Validate(makeNonce(0, 1)); err != nil {
		t.Fatalf("expected first nonce unique, got %v", err)
	}
	// Duplicate nonce
	if err := sc.Validate(makeNonce(0, 1)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate nonce error, got %v", err)
	}
	// Lower nonce with same high
	if err := sc.Validate(makeNonce(0, 0)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected lower nonce error, got %v", err)
	}
	// Higher low with same high
	if err := sc.Validate(makeNonce(0, 2)); err != nil {
		t.Fatalf("expected higher nonce unique, got %v", err)
	}
	// Higher high resets counter
	if err := sc.Validate(makeNonce(1, 0)); err != nil {
		t.Fatalf("expected higher high unique, got %v", err)
	}
	// Duplicate after reset
	if err := sc.Validate(makeNonce(1, 0)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate nonce error after reset, got %v", err)
	}
}

func TestSliding64AdvanceSmallShift(t *testing.T) {
	s := NewSliding64()
	// Initial advance
	if err := s.Validate(makeNonce(0, 10)); err != nil {
		t.Fatalf("initial advance failed: %v", err)
	}
	// Advance with small shift (<64)
	if err := s.Validate(makeNonce(0, 15)); err != nil {
		t.Fatalf("small shift advance failed: %v", err)
	}
}

func TestSliding64AdvanceReset(t *testing.T) {
	s := NewSliding64()
	// Advance to a high low value
	if err := s.Validate(makeNonce(0, 100)); err != nil {
		t.Fatalf("advance to 100 failed: %v", err)
	}
	// Advance with large shift (>=64) should reset bitmap
	if err := s.Validate(makeNonce(0, 200)); err != nil {
		t.Fatalf("large shift advance failed: %v", err)
	}
}

func TestSliding64WindowBehavior(t *testing.T) {
	s := NewSliding64()
	// Set max to 100
	if err := s.Validate(makeNonce(0, 100)); err != nil {
		t.Fatalf("advance to 100 failed: %v", err)
	}
	// Within window (low=99)
	if err := s.Validate(makeNonce(0, 99)); err != nil {
		t.Fatalf("window accept failed: %v", err)
	}
	// Duplicate within window
	if err := s.Validate(makeNonce(0, 99)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate nonce error in window, got %v", err)
	}
	// Too old (low = max - 64)
	if err := s.Validate(makeNonce(0, 36)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected too old nonce error, got %v", err)
	}
}

func TestSliding64SeparateHighs(t *testing.T) {
	s := NewSliding64()
	// nonce with high=1
	if err := s.Validate(makeNonce(1, 50)); err != nil {
		t.Fatalf("high=1 advance failed: %v", err)
	}
	// Same low, different high=2
	if err := s.Validate(makeNonce(2, 50)); err != nil {
		t.Fatalf("high=2 advance failed: %v", err)
	}
}

func TestSliding64_BigJumpMarksCurrent(t *testing.T) {
	v := NewSliding64()
	var n [12]byte

	// low = 1
	binary.BigEndian.PutUint64(n[0:8], 1)
	if err := v.Validate(n); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// big jump: low = 1 + 100
	binary.BigEndian.PutUint64(n[0:8], 101)
	if err := v.Validate(n); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// replay the same 101 must be rejected
	if err := v.Validate(n); err == nil {
		t.Fatalf("expected ErrNonUniqueNonce after big jump replay")
	}
}
