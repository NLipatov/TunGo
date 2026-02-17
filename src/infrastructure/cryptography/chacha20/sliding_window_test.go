package chacha20

import (
	"encoding/binary"
	"errors"
	"testing"
)

func makeNonce(high uint16, low uint64) [12]byte {
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[0:8], low)
	binary.BigEndian.PutUint16(nonce[8:10], high)
	// epoch at [10:12] defaults to 0
	return nonce
}

func applyNonce(s *SlidingWindow, nonce [12]byte) error {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint16(nonce[8:10])
	if err := s.Check(low, high); err != nil {
		return err
	}
	s.Accept(low, high)
	return nil
}

func TestSlidingWindowAdvanceSmallShift(t *testing.T) {
	s := NewSlidingWindow()
	// Initial advance
	if err := applyNonce(s, makeNonce(0, 10)); err != nil {
		t.Fatalf("initial advance failed: %v", err)
	}
	// Advance with small shift (<64)
	if err := applyNonce(s, makeNonce(0, 15)); err != nil {
		t.Fatalf("small shift advance failed: %v", err)
	}
}

func TestSlidingWindowAdvanceReset(t *testing.T) {
	s := NewSlidingWindow()
	// Advance to a high low value
	if err := applyNonce(s, makeNonce(0, 100)); err != nil {
		t.Fatalf("advance to 100 failed: %v", err)
	}
	// Advance with large shift (>=64) should reset bitmap
	if err := applyNonce(s, makeNonce(0, 200)); err != nil {
		t.Fatalf("large shift advance failed: %v", err)
	}
}

func TestSlidingWindowWindowBehavior(t *testing.T) {
	s := NewSlidingWindow()
	const windowSize = slidingWindowWords * 64
	const max = windowSize + 200
	// Set max
	if err := applyNonce(s, makeNonce(0, max)); err != nil {
		t.Fatalf("advance to %d failed: %v", max, err)
	}
	// Within window (low=max-1)
	if err := applyNonce(s, makeNonce(0, max-1)); err != nil {
		t.Fatalf("window accept failed: %v", err)
	}
	// Duplicate within window
	if err := applyNonce(s, makeNonce(0, max-1)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate nonce error in window, got %v", err)
	}
	// Too old (low = max - windowSize)
	tooOld := uint64(max - windowSize)
	if err := applyNonce(s, makeNonce(0, tooOld)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected too old nonce error, got %v", err)
	}
}

func TestSlidingWindowSeparateHighs(t *testing.T) {
	s := NewSlidingWindow()
	// nonce with high=1
	if err := applyNonce(s, makeNonce(1, 50)); err != nil {
		t.Fatalf("high=1 advance failed: %v", err)
	}
	// Same low, different high=2
	if err := applyNonce(s, makeNonce(2, 50)); err != nil {
		t.Fatalf("high=2 advance failed: %v", err)
	}
}

func TestSlidingWindow_BigJumpMarksCurrent(t *testing.T) {
	v := NewSlidingWindow()
	var n [12]byte

	// low = 1
	binary.BigEndian.PutUint64(n[0:8], 1)
	if err := applyNonce(v, n); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// big jump: low = 1 + 100
	binary.BigEndian.PutUint64(n[0:8], 101)
	if err := applyNonce(v, n); err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	// replay the same 101 must be rejected
	if err := applyNonce(v, n); err == nil {
		t.Fatalf("expected ErrNonUniqueNonce after big jump replay")
	}
}

func TestSlidingWindow_CheckAcceptAndZeroize(t *testing.T) {
	s := NewSlidingWindow()

	// Check on unknown high should accept.
	if err := s.Check(10, 1); err != nil {
		t.Fatalf("expected check accept for new high, got %v", err)
	}

	// Accept creates window.
	s.Accept(10, 1)
	if len(s.wins) != 1 {
		t.Fatalf("expected 1 window, got %d", len(s.wins))
	}

	// Check duplicate should reject.
	if err := s.Check(10, 1); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
	// Check with low > current max should accept.
	if err := s.Check(11, 1); err != nil {
		t.Fatalf("expected high nonce accept, got %v", err)
	}
	// Move max far enough for too-old check.
	const windowSize = slidingWindowWords * 64
	s.Accept(windowSize+100, 1)
	// Check too old should reject.
	if err := s.Check(99, 1); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("expected too-old rejection, got %v", err)
	}
	// Check unseen in-window should accept.
	if err := s.Check(windowSize+99, 1); err != nil {
		t.Fatalf("expected unseen in-window accept, got %v", err)
	}
	// Accept in-window unseen.
	s.Accept(windowSize+99, 1)
	// Accept large shift path.
	s.Accept(windowSize+200, 1)
	// Accept too-old no-op path.
	beforeMax := s.wins[0].max
	beforeBitmap := s.wins[0].bitmap
	s.Accept(200, 1)
	if s.wins[0].max != beforeMax || s.wins[0].bitmap != beforeBitmap {
		t.Fatal("expected too-old Accept to be no-op")
	}

	// Fill and exceed cap in Accept (eviction path).
	s.Accept(1, 2)
	s.Accept(1, 3)
	s.Accept(1, 4)
	s.Accept(1, 5) // should evict one high window
	if len(s.wins) != 4 {
		t.Fatalf("expected capped windows=4, got %d", len(s.wins))
	}

	// Zeroize must clear all state.
	s.Zeroize()
	if len(s.wins) != 0 {
		t.Fatalf("expected zeroized windows, got %d", len(s.wins))
	}
}

func TestSlidingWindow_EvictsOldestAtCapacity(t *testing.T) {
	s := NewSlidingWindow()
	for high := uint16(1); high <= 4; high++ {
		if err := applyNonce(s, makeNonce(high, 1)); err != nil {
			t.Fatalf("validate high=%d failed: %v", high, err)
		}
	}
	if len(s.wins) != 4 {
		t.Fatalf("expected full windows, got %d", len(s.wins))
	}
	// This should trigger eviction path when len == cap and high is new.
	if err := applyNonce(s, makeNonce(5, 1)); err != nil {
		t.Fatalf("validate high=5 failed: %v", err)
	}
	if len(s.wins) != 4 {
		t.Fatalf("expected windows capped at 4, got %d", len(s.wins))
	}
}

// --- shiftBitmap edge cases ---

func TestShiftBitmap_ZeroShift(t *testing.T) {
	var b [slidingWindowWords]uint64
	b[0] = 0xDEAD
	b[1] = 0xBEEF
	orig := b
	shiftBitmap(&b, 0)
	if b != orig {
		t.Fatal("zero shift must be no-op")
	}
}

func TestShiftBitmap_ExactWordShift(t *testing.T) {
	var b [slidingWindowWords]uint64
	b[0] = 0xAAAA
	b[1] = 0xBBBB
	shiftBitmap(&b, 64) // shift by exactly 1 word
	if b[0] != 0 {
		t.Fatalf("word[0] should be zero after 1-word shift, got %#x", b[0])
	}
	if b[1] != 0xAAAA {
		t.Fatalf("word[1] should contain old word[0], got %#x", b[1])
	}
	if b[2] != 0xBBBB {
		t.Fatalf("word[2] should contain old word[1], got %#x", b[2])
	}
}

func TestShiftBitmap_BitOnlyShift(t *testing.T) {
	var b [slidingWindowWords]uint64
	b[0] = 1 // bit 0 set
	shiftBitmap(&b, 3)
	if b[0] != 8 { // 1 << 3
		t.Fatalf("expected bit 3 set, got %#x", b[0])
	}
}

func TestShiftBitmap_CrossWordCarry(t *testing.T) {
	var b [slidingWindowWords]uint64
	b[0] = 1 << 63 // highest bit of word 0
	shiftBitmap(&b, 1)
	if b[0] != 0 {
		t.Fatalf("word[0] should be zero after carry, got %#x", b[0])
	}
	if b[1] != 1 { // carried into lowest bit of word 1
		t.Fatalf("word[1] should have carry bit, got %#x", b[1])
	}
}

func TestShiftBitmap_WordAndBitShiftCombined(t *testing.T) {
	var b [slidingWindowWords]uint64
	b[0] = 0b101 // bits 0 and 2
	shiftBitmap(&b, 64+3)
	// word shift by 1, then bit shift by 3
	if b[0] != 0 || b[1] != 0b101000 {
		t.Fatalf("combined shift failed: word[0]=%#x word[1]=%#x", b[0], b[1])
	}
}

func TestShiftBitmap_FullWindowShift(t *testing.T) {
	var b [slidingWindowWords]uint64
	for i := range b {
		b[i] = ^uint64(0)
	}
	shiftBitmap(&b, slidingWindowWords*64)
	for i, w := range b {
		if w != 0 {
			t.Fatalf("word[%d] not zero after full shift: %#x", i, w)
		}
	}
}

func TestShiftBitmap_OverflowShift(t *testing.T) {
	var b [slidingWindowWords]uint64
	for i := range b {
		b[i] = ^uint64(0)
	}
	shiftBitmap(&b, slidingWindowWords*64+100)
	for i, w := range b {
		if w != 0 {
			t.Fatalf("word[%d] not zero after overflow shift: %#x", i, w)
		}
	}
}

// --- Bitmap integrity: old nonces survive shift ---

func TestSlidingWindow_OldNoncesSurviveShift(t *testing.T) {
	s := NewSlidingWindow()

	// Accept nonces 100, 99, 98
	s.Accept(100, 0)
	s.Accept(99, 0)
	s.Accept(98, 0)

	// Advance to 110 — shift by 10, old nonces should survive
	s.Accept(110, 0)

	// 100, 99, 98 should still be seen as duplicates
	for _, low := range []uint64{100, 99, 98} {
		if err := s.Check(low, 0); !errors.Is(err, ErrNonUniqueNonce) {
			t.Fatalf("nonce %d should be duplicate after shift, got %v", low, err)
		}
	}
	// 97 was never accepted — should be allowed
	if err := s.Check(97, 0); err != nil {
		t.Fatalf("nonce 97 should be accepted, got %v", err)
	}
}

func TestSlidingWindow_OldNoncesSurviveCrossWordShift(t *testing.T) {
	s := NewSlidingWindow()

	s.Accept(10, 0)
	// Jump across a word boundary
	s.Accept(10+65, 0)

	// 10 is now at offset 65 in bitmap (word 1, bit 1) — must still be duplicate
	if err := s.Check(10, 0); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatal("nonce 10 should survive cross-word shift")
	}
}

func TestSlidingWindow_OldNoncesExpireAfterLargeShift(t *testing.T) {
	s := NewSlidingWindow()

	s.Accept(10, 0)
	// Jump beyond window size — nonce 10 should be evicted
	s.Accept(10+slidingWindowWords*64+1, 0)

	// 10 is now too old
	if err := s.Check(10, 0); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatal("nonce 10 should be too old after window-sized shift")
	}
}

// --- Boundary values ---

func TestSlidingWindow_LowZero(t *testing.T) {
	s := NewSlidingWindow()
	// First nonce with low=0 should be accepted (max starts at 0, but bitmap is empty)
	if err := applyNonce(s, makeNonce(0, 0)); err != nil {
		t.Fatalf("low=0 first use should be accepted, got %v", err)
	}
	// Duplicate
	if err := applyNonce(s, makeNonce(0, 0)); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("low=0 replay should be rejected, got %v", err)
	}
}

func TestSlidingWindow_ExactMaxDuplicate(t *testing.T) {
	s := NewSlidingWindow()
	s.Accept(50, 0)
	// low == max exactly — should be duplicate (bitmap[0] bit 0)
	if err := s.Check(50, 0); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatal("low == max should be duplicate")
	}
}

func TestSlidingWindow_WindowEdgeBoundary(t *testing.T) {
	const windowSize = slidingWindowWords * 64
	s := NewSlidingWindow()
	s.Accept(windowSize+10, 0)

	// Last valid position in window: max - (windowSize - 1) = 11
	lastValid := uint64(windowSize + 10 - (windowSize - 1))
	if err := s.Check(lastValid, 0); err != nil {
		t.Fatalf("last valid in-window nonce should be accepted, got %v", err)
	}

	// First too-old position: max - windowSize = 10
	firstTooOld := uint64(windowSize + 10 - windowSize)
	if err := s.Check(firstTooOld, 0); !errors.Is(err, ErrNonUniqueNonce) {
		t.Fatalf("first too-old nonce should be rejected, got %v", err)
	}
}

// --- Helper vs manual Check+Accept equivalence ---

func TestSlidingWindow_ApplyNonceEqualsCheckAccept(t *testing.T) {
	// Apply the same sequence to both helper-based and manual windows.
	// They must produce identical results.
	nonces := []struct {
		low  uint64
		high uint16
	}{
		{1, 0}, {5, 0}, {3, 0}, {3, 0}, // advance, advance, in-window, duplicate
		{1, 1}, {200, 0}, {4, 0}, // new high, big jump, too-old
		{199, 0}, {199, 0}, // in-window, duplicate
	}

	vw := NewSlidingWindow()  // uses applyNonce helper
	caw := NewSlidingWindow() // uses Check+Accept

	for i, n := range nonces {
		vErr := applyNonce(vw, makeNonce(n.high, n.low))
		cErr := caw.Check(n.low, n.high)
		if cErr == nil {
			caw.Accept(n.low, n.high)
		}

		vOk := vErr == nil
		cOk := cErr == nil
		if vOk != cOk {
			t.Fatalf("step %d (low=%d high=%d): applyNonce=%v, Check=%v",
				i, n.low, n.high, vErr, cErr)
		}
	}
}
