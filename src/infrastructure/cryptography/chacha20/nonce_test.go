package chacha20

import (
	"encoding/binary"
	"testing"
)

// TestNonceInitialization ensures that the nonce is initialized with zero values.
func TestNonceInitialization(t *testing.T) {
	const epoch = Epoch(7)
	nonce := NewNonce(epoch)
	if nonce.counterLow != 0 || nonce.counterHigh != 0 || nonce.epoch != epoch {
		t.Errorf("Expected low=0 high=0 epoch=%d, got low=%d, high=%d, epoch=%d", epoch, nonce.counterLow, nonce.counterHigh, nonce.epoch)
	}
}

// TestNonceIncrement checks that incrementNonce works correctly without overflow.
func TestNonceIncrement(t *testing.T) {
	nonce := NewNonce(0)
	for i := 1; i <= 5; i++ {
		if err := nonce.incrementNonce(); err != nil {
			t.Fatalf("incrementNonce returned error: %v", err)
		}
		if nonce.counterLow != uint64(i) || nonce.counterHigh != 0 {
			t.Errorf("After %d increments, expected low=%d, high=0, got low=%d, high=%d", i, i, nonce.counterLow, nonce.counterHigh)
		}
	}
}

// TestNonceLowOverflow checks that when low overflows, high increments and low resets.
func TestNonceLowOverflow(t *testing.T) {
	nonce := NewNonce(0)
	nonce.counterLow = ^uint64(0) // Set low to maximum value.
	if err := nonce.incrementNonce(); err != nil {
		t.Fatalf("incrementNonce returned error: %v", err)
	}
	if nonce.counterLow != 0 || nonce.counterHigh != 1 {
		t.Errorf("Expected low=0 and high=1 after low overflow, got low=%d, high=%d", nonce.counterLow, nonce.counterHigh)
	}
}

// TestNonceHighOverflow checks that when both low and high are at maximum values, an error is returned.
func TestNonceHighOverflow(t *testing.T) {
	nonce := NewNonce(0)
	nonce.counterLow = ^uint64(0)
	nonce.counterHigh = ^uint16(0)
	err := nonce.incrementNonce()
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
	const epoch = Epoch(0x1234)
	nonce := NewNonce(epoch)
	nonce.counterLow = 0x1122334455667788
	nonce.counterHigh = 0x99AA

	// Prepare a 12-byte buffer.
	buffer := make([]byte, 12)
	encoded := nonce.Encode(buffer)

	// Build expected result.
	expected := make([]byte, 12)
	binary.BigEndian.PutUint16(expected[0:2], uint16(epoch))
	binary.BigEndian.PutUint16(expected[2:4], nonce.counterHigh)
	binary.BigEndian.PutUint64(expected[4:12], nonce.counterLow)

	// Compare encoded bytes.
	for i := range expected {
		if encoded[i] != expected[i] {
			t.Errorf("Encoded byte mismatch at index %d: expected %02x, got %02x", i, expected[i], encoded[i])
		}
	}
}
