package hmac

import (
	"bytes"
	"testing"
)

func TestCryptoHMAC_GenerateAndVerify_Success(t *testing.T) {
	key := []byte("my-secret-key")
	data := []byte("hello world")

	h := NewHMAC(key)

	mac, err := h.Generate(data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(mac) == 0 {
		t.Fatal("Expected non-empty mac")
	}

	if err := h.Verify(data, mac); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
}

func TestCryptoHMAC_Verify_Fail(t *testing.T) {
	key := []byte("super-secret")
	data := []byte("payload")
	gH := NewHMAC(key)

	mac, err := gH.Generate(data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	vH := NewHMAC(key)

	badMac := make([]byte, len(mac))
	copy(badMac, mac)
	badMac[0] ^= 0xFF // flip 1st byte
	if err := vH.Verify(data, badMac); err == nil {
		t.Fatalf("Expected ErrUnexpectedSignature on tampered mac")
	}

	badData := make([]byte, len(data))
	copy(badData, data)
	badData[0] ^= 0xFF // flip 1st byte
	if err := vH.Verify(badData, mac); err == nil {
		t.Fatalf("Expected ErrUnexpectedSignature on tampered data")
	}
}

func TestCryptoHMAC_EmptySecretOrData(t *testing.T) {
	h := NewHMAC(nil)
	data := []byte("anything")
	mac, err := h.Generate(data)
	if err != nil {
		t.Fatalf("Generate failed with empty secret: %v", err)
	}
	if err := h.Verify(data, mac); err != nil {
		t.Fatalf("Verify failed with empty secret: %v", err)
	}

	key := []byte("nonempty")
	h = NewHMAC(key)
	mac, err = h.Generate(nil)
	if err != nil {
		t.Fatalf("Generate failed with empty data: %v", err)
	}
	if err := h.Verify(nil, mac); err != nil {
		t.Fatalf("Verify failed with empty data: %v", err)
	}
}

func TestCryptoHMAC_ReuseInstance(t *testing.T) {
	key := []byte("reuse-key")
	generatorOne := NewHMAC(key)
	generatorTwo := NewHMAC(key)
	data1 := []byte("foo")
	data2 := []byte("bar")

	mac1, err := generatorOne.Generate(data1)
	if err != nil {
		t.Fatalf("Generate 1 failed: %v", err)
	}
	mac2, err := generatorTwo.Generate(data2)
	if err != nil {
		t.Fatalf("Generate 2 failed: %v", err)
	}
	if bytes.Equal(mac1, mac2) {
		t.Error("MACs for different data must differ")
	}

	verifierOne := NewHMAC(key)
	verifierTwo := NewHMAC(key)

	if err := verifierOne.Verify(data1, mac1); err != nil {
		t.Errorf("Verify data1 failed: %v", err)
	}
	if err := verifierTwo.Verify(data2, mac2); err != nil {
		t.Errorf("Verify data2 failed: %v", err)
	}
}
func TestCryptoHMAC_MacSize(t *testing.T) {
	key := []byte("mac-size-key")
	data := []byte("data")
	h := NewHMAC(key)
	mac, err := h.Generate(data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(mac) != 32 {
		t.Fatalf("Expected MAC size 32, got %d", len(mac))
	}
}

func TestCryptoHMAC_WrongLengthSignature(t *testing.T) {
	key := []byte("mac-size-key")
	data := []byte("data")
	h := NewHMAC(key)
	mac, _ := h.Generate(data)

	short := mac[:len(mac)-1]
	long := append(mac, 1)
	if err := h.Verify(data, short); err == nil {
		t.Fatal("Expected error for short signature")
	}
	if err := h.Verify(data, long); err == nil {
		t.Fatal("Expected error for long signature")
	}
}

func TestCryptoHMAC_DifferentKeysDifferentMac(t *testing.T) {
	data := []byte("zzz")
	h1 := NewHMAC([]byte("a"))
	h2 := NewHMAC([]byte("b"))
	m1, _ := h1.Generate(data)
	m2, _ := h2.Generate(data)
	if bytes.Equal(m1, m2) {
		t.Error("MACs for different keys must differ")
	}
}

func TestCryptoHMAC_RepeatedUseDoesNotCorrupt(t *testing.T) {
	key := []byte("reuse")
	h := NewHMAC(key)
	for i := 0; i < 10; i++ {
		data := []byte{byte(i)}
		mac, err := h.Generate(data)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if err := h.Verify(data, mac); err != nil {
			t.Fatalf("Verify failed on iteration %d: %v", i, err)
		}
	}
}

func TestCryptoHMAC_BulkVerify(t *testing.T) {
	key := []byte("bulk-secret")

	type sample struct {
		data []byte
		mac  []byte
	}

	// samples imitate client generated HMACs
	samples := make([]sample, 5)
	for i := 0; i < 5; i++ {
		d := []byte{byte('a' + i), byte(i)}
		generator := NewHMAC(key)
		mac, err := generator.Generate(d)
		if err != nil {
			t.Fatalf("Generate failed for %q: %v", d, err)
		}
		samples[i] = sample{data: d, mac: mac}
	}

	// verifier imitates server HMAC instance that is verifying samples
	verifier := NewHMAC(key)
	for j, s := range samples {
		if verifyErr := verifier.Verify(s.data, s.mac); verifyErr != nil {
			t.Errorf("Verify failed for sample %d (%q): %v", j, s.data, verifyErr)
		}
	}
}
