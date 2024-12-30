package chacha20

import (
	"encoding/binary"
	"encoding/hex"
	"sync"
	"testing"
)

// TestNonceInitialization ensures that nonce is initialized with zero values for high and low
func TestNonceInitialization(t *testing.T) {
	nonce := NewNonce()
	if nonce.low != 0 || nonce.high != 0 {
		t.Errorf("Expected low=0 and high=0, got low=%d, high=%d", nonce.low, nonce.high)
	}
}

// TestNonceIncrement checks no-overflow increment call
func TestNonceIncrement(t *testing.T) {
	nonce := NewNonce()
	for i := 1; i <= 5; i++ {
		err := nonce.incrementNonce()
		if err != nil {
			t.Fatalf("incrementNonce returned error: %v", err)
		}

		if nonce.low != uint64(i) || nonce.high != 0 {
			t.Errorf("After %d increments, expected low=%d, high=0, got low=%d, high=%d", i, i, nonce.low, nonce.high)
		}
	}
}

// TestNonceLowOverflow checks low-overflow increment call
func TestNonceLowOverflow(t *testing.T) {
	nonce := NewNonce()
	nonce.low = ^uint64(0)

	err := nonce.incrementNonce()
	if err != nil {
		t.Fatalf("incrementNonce returned error: %v", err)
	}

	if nonce.low != 0 || nonce.high != 1 {
		t.Errorf("Expected low=0 and high=1 after overflow, got low=%d, high=%d", nonce.low, nonce.high)
	}
}

// TestNonceHighOverflow checks high-and-low-overflow increment call
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

// TestNonceHash checks correctness of Hash function
func TestNonceHash(t *testing.T) {
	nonce := NewNonce()
	nonce.low = 0x1122334455667788
	nonce.high = 0x99AABBCC

	expectedHash := hex.EncodeToString(nonce.Encode())

	hash := nonce.Hash()
	if hash != expectedHash {
		t.Errorf("Expected hash '%s', got '%s'", expectedHash, hash)
	}
}

// TestNonceEncode checks correctness of Encode function
func TestNonceEncode(t *testing.T) {
	nonce := NewNonce()
	nonce.low = 0x1122334455667788
	nonce.high = 0x99AABBCC

	expectedBytes := make([]byte, 12)
	binary.BigEndian.PutUint64(expectedBytes[:8], nonce.low)
	binary.BigEndian.PutUint32(expectedBytes[8:], nonce.high)

	encoded := nonce.Encode()
	for i := range expectedBytes {
		if encoded[i] != expectedBytes[i] {
			t.Errorf("Encoded bytes do not match expected bytes")
			break
		}
	}
}

// TestNonceConcurrentIncrement checks that increment handles concurrent invocations correctly
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
				err := nonce.incrementNonce()
				if err != nil {
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
