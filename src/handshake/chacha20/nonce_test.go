package chacha20

import (
	"encoding/binary"
	"encoding/hex"
	"sync"
	"testing"
)

// TestNonceInitialization ensures that nonce is initialized with 0valued high and low
func TestNonceInitialization(t *testing.T) {
	nonce := NewNonce()
	if nonce.Low != 0 || nonce.High != 0 {
		t.Errorf("Expected Low=0 and High=0, got Low=%d, High=%d", nonce.Low, nonce.High)
	}
}

// TestNonceIncrement checks no-overflow increment call
func TestNonceIncrement(t *testing.T) {
	nonce := NewNonce()
	for i := 1; i <= 5; i++ {
		nonceBytes, highVal, lowVal, err := nonce.incrementNonce()
		if err != nil {
			t.Fatalf("incrementNonce returned error: %v", err)
		}

		if lowVal != uint64(i) || highVal != 0 {
			t.Errorf("After %d increments, expected Low=%d, High=0, got Low=%d, High=%d", i, i, lowVal, highVal)
		}

		if len(nonceBytes) != 12 {
			t.Errorf("Expected nonceBytes length to be 12, got %d", len(nonceBytes))
		}
	}
}

// TestNonceLowOverflow checks low-overflow increment call
func TestNonceLowOverflow(t *testing.T) {
	nonce := NewNonce()
	nonce.Low = ^uint64(0)

	_, highVal, lowVal, err := nonce.incrementNonce()
	if err != nil {
		t.Fatalf("incrementNonce returned error: %v", err)
	}

	if lowVal != 0 || highVal != 1 {
		t.Errorf("Expected Low=0 and High=1 after overflow, got Low=%d, High=%d", lowVal, highVal)
	}
}

// TestNonceHighOverflow checks high-and-low-overflow increment call
func TestNonceHighOverflow(t *testing.T) {
	nonce := NewNonce()
	nonce.Low = ^uint64(0)
	nonce.High = ^uint32(0)

	_, _, _, err := nonce.incrementNonce()
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
	nonce.Low = 0x1122334455667788
	nonce.High = 0x99AABBCC

	expectedNonce := Encode(nonce.High, nonce.Low)
	expectedHash := hex.EncodeToString(expectedNonce[:])

	hash := nonce.Hash()
	if hash != expectedHash {
		t.Errorf("Expected hash '%s', got '%s'", expectedHash, hash)
	}
}

// TestEncode checks correctness of Encode function
func TestEncode(t *testing.T) {
	high := uint32(0x12345678)
	low := uint64(0x9ABCDEF012345678)

	expectedBytes := make([]byte, 12)
	binary.BigEndian.PutUint64(expectedBytes[:8], low)
	binary.BigEndian.PutUint32(expectedBytes[8:], high)

	nonceBytes := Encode(high, low)

	if string(nonceBytes[:]) != string(expectedBytes) {
		t.Errorf("Encoded bytes do not match expected bytes")
	}
}

// TestNonceConcurrentIncrement checks that increment is handling concurrent invocation in correct way
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
				_, _, _, err := nonce.incrementNonce()
				if err != nil {
					t.Errorf("incrementNonce returned error: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	expectedLow := uint64(numGoroutines * incrementsPerGoroutine)
	if nonce.Low != expectedLow {
		t.Errorf("Expected Low=%d after concurrent increments, got Low=%d", expectedLow, nonce.Low)
	}
}
