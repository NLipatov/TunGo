package chacha20

import (
	"errors"
	"testing"
)

func TestReplayWindowInitialAndDuplicate(t *testing.T) {
	w := &ReplayWindow{}

	// First sequence should be accepted
	if err := w.Validate(5); err != nil {
		t.Fatalf("expected first seq unique, got %v", err)
	}
	// Duplicate sequence should be rejected
	if err := w.Validate(5); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate seq error, got %v", err)
	}
}

func TestReplayWindowSmallShiftAndWindow(t *testing.T) {
	w := &ReplayWindow{}
	// Advance to max=10
	if err := w.Validate(10); err != nil {
		t.Fatalf("advance to 10 failed: %v", err)
	}
	// Small shift (<64): seq=15 accepted
	if err := w.Validate(15); err != nil {
		t.Fatalf("small shift advance failed: %v", err)
	}
	// Within window: lower seq=12 accepted
	if err := w.Validate(12); err != nil {
		t.Fatalf("window accept failed: %v", err)
	}
	// Duplicate within window: seq=12 rejected
	if err := w.Validate(12); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate seq error in window, got %v", err)
	}
}

func TestReplayWindowLargeShiftReset(t *testing.T) {
	w := &ReplayWindow{}
	// Advance to initial
	if err := w.Validate(0); err != nil {
		t.Fatalf("initial advance failed: %v", err)
	}
	// Large shift (>=64): seq=64 resets bitmap
	if err := w.Validate(64); err != nil {
		t.Fatalf("large shift advance failed: %v", err)
	}
	// After reset, old seq=0 is too old (max-seq=64) and should be rejected
	if err := w.Validate(0); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected too old seq error after reset, got %v", err)
	}
}

func TestReplayWindowTooOld(t *testing.T) {
	w := &ReplayWindow{}
	// Set max=100
	if err := w.Validate(100); err != nil {
		t.Fatalf("advance to 100 failed: %v", err)
	}
	// Too old (seq = max-64 = 36): rejected
	if err := w.Validate(36); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected too old seq error, got %v", err)
	}
}
