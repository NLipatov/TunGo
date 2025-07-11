package obfuscation

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/hmac"
)

type badHMACImpl struct{}

func (b *badHMACImpl) Verify(_, _ []byte) error {
	panic("not implemented")
}

func (b *badHMACImpl) ResultSize() int {
	panic("not implemented")
}

func (b *badHMACImpl) Generate(_ []byte) ([]byte, error) { return nil, errors.New("fail") }

func mustRandBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func testObfuscator(min, max int) *ChaCha20Obfuscator {
	key := sha256.Sum256([]byte("test-key"))
	psk := []byte("test-psk")
	hmacKey := sha256.Sum256([]byte("test-hmac"))
	hmacInst := hmac.NewHMAC(hmacKey[:])
	return NewChaCha20Obfuscator(
		key[:], psk, hmacInst, chacha20.NewSliding64(), min, max,
	).(*ChaCha20Obfuscator)
}

func TestChaCha20Obfuscator_Roundtrip(t *testing.T) {
	obf := testObfuscator(60, 120)
	plain := mustRandBytes(40)

	obfData, err := obf.Obfuscate(plain)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	dec, err := obf.Deobfuscate(obfData)
	if err != nil {
		t.Fatalf("Deobfuscate: %v", err)
	}
	if !bytes.Equal(plain, dec) {
		t.Error("Decoded data mismatch")
	}
}

func TestChaCha20Obfuscator_Corrupted(t *testing.T) {
	obf := testObfuscator(80, 120)
	plain := mustRandBytes(30)
	obfData, err := obf.Obfuscate(plain)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	obfData[10] ^= 0x77 // corrupt a byte

	dec, err := obf.Deobfuscate(obfData)
	if err != nil && dec != nil && len(dec) != 0 {
		t.Error("On error, decrypted data should be nil or empty")
	}
}

func TestChaCha20Obfuscator_GarbageInput(t *testing.T) {
	obf := testObfuscator(32, 64)
	garbage := mustRandBytes(90)
	dec, err := obf.Deobfuscate(garbage)
	if err == nil {
		t.Error("Random input should not decrypt")
	}
	if dec != nil {
		t.Error("Decrypted data on garbage should be nil")
	}
}

func TestChaCha20Obfuscator_MinGreaterThanMax(t *testing.T) {
	obf := testObfuscator(100, 80)
	plain := mustRandBytes(10)
	_, err := obf.Obfuscate(plain)
	if err == nil {
		t.Error("Expected error when minLen > maxLen")
	}
}

func TestChaCha20Obfuscator_NoPaddingNeeded(t *testing.T) {
	obf := testObfuscator(10, 10)
	plain := mustRandBytes(15)
	obfData, err := obf.Obfuscate(plain)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	if len(obfData) < 15 {
		t.Errorf("No padding should be needed, but got %d bytes", len(obfData))
	}
}

func TestChaCha20Obfuscator_PaddingRandomizesLength(t *testing.T) {
	obf := testObfuscator(64, 128)
	plain := mustRandBytes(16)
	lengths := make(map[int]struct{})
	for i := 0; i < 8; i++ {
		obfData, err := obf.Obfuscate(plain)
		if err != nil {
			t.Fatalf("Obfuscate: %v", err)
		}
		lengths[len(obfData)] = struct{}{}
	}
	if len(lengths) < 2 {
		t.Error("Packet length is not randomized with padding")
	}
}

func TestChaCha20Obfuscator_LargeData(t *testing.T) {
	obf := testObfuscator(50, 100)
	plain := mustRandBytes(200)
	obfData, err := obf.Obfuscate(plain)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	dec, err := obf.Deobfuscate(obfData)
	if err != nil {
		t.Fatalf("Deobfuscate: %v", err)
	}
	if !bytes.Equal(plain, dec) {
		t.Error("Decoded data mismatch on large input")
	}
}

func TestChaCha20Obfuscator_PaddingEdgeCases(t *testing.T) {
	// Case: n == max
	obf := testObfuscator(32, 64)
	data := mustRandBytes(64)
	out, err := obf.addPadding(data)
	if err != nil {
		t.Fatalf("addPadding: %v", err)
	}
	if !bytes.Equal(data, out) {
		t.Error("addPadding should not alter if n == max")
	}
	// Case: min==max, n < min
	obf2 := testObfuscator(40, 40)
	data2 := mustRandBytes(16)
	out2, err := obf2.addPadding(data2)
	if err != nil {
		t.Fatalf("addPadding: %v", err)
	}
	if len(out2) != 40 {
		t.Error("addPadding wrong output length when min==max")
	}
}

func TestChaCha20Obfuscator_deterministicOffsetZero(t *testing.T) {
	obf := testObfuscator(32, 64)
	nonce := mustRandBytes(12)
	offset := obf.deterministicOffset(nonce, 0)
	if offset != 0 {
		t.Error("deterministicOffset should be 0 when maxOffset=0")
	}
}

func TestChaCha20Obfuscator_addPadding_InvalidRange(t *testing.T) {
	obf := testObfuscator(40, 30)
	data := mustRandBytes(10)
	_, err := obf.addPadding(data)
	if err == nil {
		t.Error("Expected error for addPadding when min > max")
	}
}

func TestChaCha20Obfuscator_addPadding_MaxLenReached(t *testing.T) {
	obf := testObfuscator(8, 8)
	data := mustRandBytes(8)
	out, err := obf.addPadding(data)
	if err != nil {
		t.Fatalf("addPadding: %v", err)
	}
	if len(out) != 8 || !bytes.Equal(data, out) {
		t.Error("Should return the same slice for n == maxLen")
	}
}

func TestDeobfuscate_EdgeCases(t *testing.T) {
	obf := testObfuscator(60, 120)
	// too short input
	_, err := obf.Deobfuscate([]byte{1, 2, 3})
	if err == nil {
		t.Error("Should fail on too short input")
	}
}

func TestObfuscate_EmptyData(t *testing.T) {
	obf := testObfuscator(8, 16)
	obfData, err := obf.Obfuscate(nil)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	dec, err := obf.Deobfuscate(obfData)
	if err != nil {
		t.Fatalf("Deobfuscate: %v", err)
	}
	if len(dec) != 0 {
		t.Error("Decoded empty data should be empty")
	}
}

func TestObfuscate_BadHMAC(t *testing.T) {
	// HMAC implementation that always fails
	badHMAC := &badHMACImpl{}
	obf := NewChaCha20Obfuscator(
		bytes.Repeat([]byte{1}, 32), []byte("psk"), badHMAC, chacha20.NewSliding64(), 16, 32,
	).(*ChaCha20Obfuscator)
	_, err := obf.Obfuscate([]byte("test"))
	if err == nil {
		t.Error("Expected error from bad HMAC")
	}
}
