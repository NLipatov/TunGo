package chacha20

import (
	"encoding/binary"
	"sync"
	"testing"
)

// TestNonceInitialization ensures that the nonce is initialized with zero values.
func TestNonceInitialization(t *testing.T) {
	nonce := NewNonce()
	if nonce.low != 0 || nonce.high != 0 {
		t.Errorf("Expected low=0 and high=0, got low=%d, high=%d", nonce.low, nonce.high)
	}
}

// TestNonceIncrement checks that incrementNonce works correctly without overflow.
func TestNonceIncrement(t *testing.T) {
	nonce := NewNonce()
	for i := 1; i <= 5; i++ {
		if err := nonce.incrementNonce(); err != nil {
			t.Fatalf("incrementNonce returned error: %v", err)
		}
		if nonce.low != uint64(i) || nonce.high != 0 {
			t.Errorf("After %d increments, expected low=%d, high=0, got low=%d, high=%d", i, i, nonce.low, nonce.high)
		}
	}
}

// TestNonceLowOverflow checks that when low overflows, high increments and low resets.
func TestNonceLowOverflow(t *testing.T) {
	nonce := NewNonce()
	nonce.low = ^uint64(0) // Set low to maximum value.
	if err := nonce.incrementNonce(); err != nil {
		t.Fatalf("incrementNonce returned error: %v", err)
	}
	if nonce.low != 0 || nonce.high != 1 {
		t.Errorf("Expected low=0 and high=1 after low overflow, got low=%d, high=%d", nonce.low, nonce.high)
	}
}

// TestNonceHighOverflow checks that when both low and high are at maximum values, an error is returned.
func TestNonceHighOverflow(t *testing.T) {
	nonce := NewNonce()
	nonce.low = ^uint64(0)
	nonce.high = ^uint32(0)
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
	nonce := NewNonce()
	nonce.low = 0x1122334455667788
	nonce.high = 0x99AABBCC

	// Prepare a 12-byte buffer.
	buffer := make([]byte, 12)
	encoded := nonce.Encode(buffer)

	// Build expected result.
	expected := make([]byte, 12)
	binary.BigEndian.PutUint64(expected[:8], nonce.low)
	binary.BigEndian.PutUint32(expected[8:], nonce.high)

	// Compare encoded bytes.
	for i := range expected {
		if encoded[i] != expected[i] {
			t.Errorf("Encoded byte mismatch at index %d: expected %02x, got %02x", i, expected[i], encoded[i])
		}
	}
}

// TestNonceConcurrentIncrement checks that incrementNonce handles concurrent invocations correctly.
func TestNonceConcurrentIncrement(t *testing.T) {
	nonce := NewNonce()
	var wg sync.WaitGroup
	numGoroutines := 10
	incrementsPerGoroutine := 1000

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				if err := nonce.incrementNonce(); err != nil {
					t.Errorf("incrementNonce returned error: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
	expectedLow := uint64(numGoroutines * incrementsPerGoroutine)
	if nonce.low != expectedLow || nonce.high != 0 {
		t.Errorf("Expected low=%d and high=0 after concurrent increments, got low=%d, high=%d", expectedLow, nonce.low, nonce.high)
	}
}
