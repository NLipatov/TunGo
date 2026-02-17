package mem

import (
	"testing"
	"unsafe"
)

func TestZeroBytes_ZeroesNonEmptySlice(t *testing.T) {
	buf := []byte{1, 2, 3, 4, 5}
	ZeroBytes(buf)
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("expected buf[%d] to be zero, got %d", i, b)
		}
	}
}

func TestZeroBytes_EmptyAndNilSlices(t *testing.T) {
	empty := []byte{}
	ZeroBytes(empty)

	var nilSlice []byte
	ZeroBytes(nilSlice)
}

// TestZeroBytes_NotOptimizedAway verifies the compiler does not eliminate
// the zeroing as a dead store. We pass a pointer to a heap-allocated buffer
// through noinline + unsafe to prevent the compiler from proving the memory
// is unused after zeroing.
//
// If a future Go compiler optimizes away the zeroing, this test will fail,
// signaling that ZeroBytes needs an assembly implementation.
func TestZeroBytes_NotOptimizedAway(t *testing.T) {
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = 0xAB
	}

	// Capture raw pointer before zeroing — survives regardless of slice liveness.
	ptr := unsafe.Pointer(&buf[0])

	zeroAndForget(buf)

	// Re-read memory through the raw pointer.
	zeroed := unsafe.Slice((*byte)(ptr), 64)
	for i, v := range zeroed {
		if v != 0 {
			t.Fatalf("byte[%d] = %#x after ZeroBytes — zeroing was optimized away", i, v)
		}
	}
}

//go:noinline
func zeroAndForget(b []byte) {
	ZeroBytes(b)
	// b goes out of scope here — if compiler treats ZeroBytes as dead store,
	// the memory won't be zeroed.
}
