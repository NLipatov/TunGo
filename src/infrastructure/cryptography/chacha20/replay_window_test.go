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

func TestReplayWindowCheck_Branches(t *testing.T) {
	w := &ReplayWindow{}

	// seq > max: would be accepted
	if err := w.Check(5); err != nil {
		t.Fatalf("expected check success for future seq, got %v", err)
	}

	// Build state max=100, bitmap includes seq=90.
	w.max = 100
	w.bitmap = uint64(1) << 10

	// Too old (max-seq >= 64): reject
	if err := w.Check(36); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected too-old rejection, got %v", err)
	}

	// Duplicate inside window: reject
	if err := w.Check(90); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}

	// Inside window but unseen: accept
	if err := w.Check(95); err != nil {
		t.Fatalf("expected in-window unseen seq accepted, got %v", err)
	}
}

func TestReplayWindowAccept_Branches(t *testing.T) {
	w := &ReplayWindow{}

	// Future seq, small shift (<64).
	w.Accept(5)
	if w.max != 5 {
		t.Fatalf("expected max=5, got %d", w.max)
	}
	if w.bitmap&1 == 0 {
		t.Fatal("expected newest bit set after first accept")
	}

	// Within window path (max-seq < 64).
	w.Accept(3)
	bit := uint64(1) << (w.max - 3)
	if w.bitmap&bit == 0 {
		t.Fatal("expected in-window bit to be marked")
	}

	// Future seq with large shift (>=64) resets bitmap.
	w.Accept(80)
	if w.max != 80 {
		t.Fatalf("expected max=80, got %d", w.max)
	}
	if w.bitmap != 0 {
		t.Fatalf("expected bitmap reset on large shift, got %064b", w.bitmap)
	}

	// Too-old path (max-seq >= 64): no-op.
	beforeMax, beforeBitmap := w.max, w.bitmap
	w.Accept(16) // 80-16 = 64
	if w.max != beforeMax || w.bitmap != beforeBitmap {
		t.Fatal("expected too-old accept to be no-op")
	}
}
