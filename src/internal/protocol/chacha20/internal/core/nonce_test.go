package core

import (
	"encoding/binary"
	"testing"
)

// TestNonceInitialization ensures that the nonce is initialized with zero values.
func TestNonceInitialization(t *testing.T) {
	const epoch uint16 = 7
	nonce := NewNonce(epoch)
	if nonce.CounterLow != 0 || nonce.CounterHigh != 0 || nonce.epoch != epoch {
		t.Errorf("Expected low=0 high=0 epoch=%d, got low=%d, high=%d, epoch=%d", epoch, nonce.CounterLow, nonce.CounterHigh, nonce.epoch)
	}
}

// TestNonceIncrement checks that incrementNonce works correctly without overflow.
func TestNonceIncrement(t *testing.T) {
	nonce := NewNonce(0)
	for i := 1; i <= 5; i++ {
		if err := nonce.Increment(); err != nil {
			t.Fatalf("incrementNonce returned error: %v", err)
		}
		if nonce.CounterLow != uint64(i) || nonce.CounterHigh != 0 {
			t.Errorf("After %d increments, expected low=%d, high=0, got low=%d, high=%d", i, i, nonce.CounterLow, nonce.CounterHigh)
		}
	}
}

// TestNonceLowOverflow checks that when low overflows, high increments and low resets.
func TestNonceLowOverflow(t *testing.T) {
	nonce := NewNonce(0)
	nonce.CounterLow = ^uint64(0) // Set low to maximum value.
	if err := nonce.Increment(); err != nil {
		t.Fatalf("incrementNonce returned error: %v", err)
	}
	if nonce.CounterLow != 0 || nonce.CounterHigh != 1 {
		t.Errorf("Expected low=0 and high=1 after low overflow, got low=%d, high=%d", nonce.CounterLow, nonce.CounterHigh)
	}
}

// TestNonceHighOverflow checks that when both low and high are at maximum values, an error is returned.
func TestNonceHighOverflow(t *testing.T) {
	nonce := NewNonce(0)
	nonce.CounterLow = ^uint64(0)
	nonce.CounterHigh = ^uint16(0)
	err := nonce.Increment()
	if err == nil {
		t.Fatalf("Expected error due to nonce overflow, but got nil")
	}
	expectedErr := "nonce overflow: maximum number of messages reached"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

// TestNonceEncode checks the correctness of the Encode function.
func TestNonceEncode(t *testing.T) {
	const epoch uint16 = 0x1234
	nonce := NewNonce(epoch)
	nonce.CounterLow = 0x1122334455667788
	nonce.CounterHigh = 0x99AA

	// Prepare a 12-byte buffer.
	buffer := make([]byte, 12)
	encoded := nonce.Encode(buffer)

	// Build expected result.
	expected := make([]byte, 12)
	binary.BigEndian.PutUint64(expected[0:8], nonce.CounterLow)
	binary.BigEndian.PutUint16(expected[8:10], nonce.CounterHigh)
	binary.BigEndian.PutUint16(expected[10:12], uint16(epoch))

	// Compare encoded bytes.
	for i := range expected {
		if encoded[i] != expected[i] {
			t.Errorf("Encoded byte mismatch at index %d: expected %02x, got %02x", i, expected[i], encoded[i])
		}
	}
}

func TestNoncePeekEncode_OverflowAndRollover(t *testing.T) {
	t.Run("overflow", func(t *testing.T) {
		n := NewNonce(0)
		n.CounterLow = ^uint64(0)
		n.CounterHigh = ^uint16(0)
		buf := make([]byte, 12)
		if _, err := n.PeekEncode(buf); err == nil {
			t.Fatal("expected overflow error from peekEncode")
		}
	})

	t.Run("rollover", func(t *testing.T) {
		const epoch uint16 = 9
		n := NewNonce(epoch)
		n.CounterLow = ^uint64(0)
		n.CounterHigh = 7

		buf := make([]byte, 12)
		out, err := n.PeekEncode(buf)
		if err != nil {
			t.Fatalf("peekEncode failed: %v", err)
		}
		if got := binary.BigEndian.Uint64(out[0:8]); got != 0 {
			t.Fatalf("expected low=0, got %d", got)
		}
		if got := binary.BigEndian.Uint16(out[8:10]); got != 8 {
			t.Fatalf("expected high=8, got %d", got)
		}
		if got := binary.BigEndian.Uint16(out[10:12]); got != uint16(epoch) {
			t.Fatalf("expected epoch=%d, got %d", epoch, got)
		}
	})
}
