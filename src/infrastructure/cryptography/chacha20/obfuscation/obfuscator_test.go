package obfuscation

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/hmac"
)

// --- Mock types for testing ---

var testMagic = []byte{0x13, 0x37, 0x42, 0x00}

type strictMarshaller struct {
	Data []byte
}

func (s *strictMarshaller) MarshalBinary() ([]byte, error) {
	return append(append([]byte{}, testMagic...), s.Data...), nil
}

func (s *strictMarshaller) UnmarshalBinary(b []byte) error {
	if len(b) < len(testMagic) || !bytes.Equal(b[:len(testMagic)], testMagic) {
		return errors.New("bad magic prefix")
	}
	if len(b[len(testMagic):]) != len(s.Data) {
		return errors.New("wrong data size")
	}
	copy(s.Data, b[len(testMagic):])
	return nil
}

type unstrictMarshaller struct {
	Data []byte
}

func (d *unstrictMarshaller) MarshalBinary() ([]byte, error) {
	return d.Data, nil
}
func (d *unstrictMarshaller) UnmarshalBinary(b []byte) error {
	d.Data = make([]byte, len(b))
	copy(d.Data, b)
	return nil
}

type failingMarshaller struct{}

func (f *failingMarshaller) MarshalBinary() ([]byte, error) { return nil, errors.New("fail") }
func (f *failingMarshaller) UnmarshalBinary([]byte) error   { return errors.New("fail") }

// --- Helper ---

func newTestObfuscator[T application.ObfuscatableData](minLen, maxLen int) application.Obfuscator[T] {
	key := sha256.Sum256([]byte("test-key"))
	psk := []byte("psk-test-xyz123456789")
	hmacKey := sha256.Sum256([]byte("test-hmac-key"))
	hmacImpl := hmac.NewHMAC(hmacKey[:])
	return NewChaCha20Obfuscator[T](
		key[:], psk, hmacImpl, chacha20.NewSliding64(), minLen, maxLen,
	)
}

// --- Tests ---

func TestChaCha20Obfuscator_Roundtrip(t *testing.T) {
	orig := &unstrictMarshaller{Data: make([]byte, 64)}
	_, _ = rand.Read(orig.Data)
	obf := newTestObfuscator[*unstrictMarshaller](60, 120)

	enc, err := obf.Obfuscate(orig)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}

	// ВАЖНО: размер должен совпадать
	_ = &unstrictMarshaller{Data: make([]byte, 64)}
	obfRecv := newTestObfuscator[*unstrictMarshaller](60, 120)
	got, err := obfRecv.Deobfuscate(enc)
	if err != nil {
		t.Fatalf("Deobfuscate: %v", err)
	}
	if !bytes.Equal(orig.Data, got.Data) {
		t.Error("Obfuscator did not roundtrip the data")
	}
}

func TestChaCha20Obfuscator_Corrupted(t *testing.T) {
	orig := &strictMarshaller{Data: make([]byte, 32)}
	_, _ = rand.Read(orig.Data)
	obf := newTestObfuscator[*strictMarshaller](40, 100)

	enc, err := obf.Obfuscate(orig)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	enc[10] ^= 0x55 // портили пакет

	_ = &strictMarshaller{Data: make([]byte, 32)}
	obfRecv := newTestObfuscator[*strictMarshaller](40, 100)
	_, err = obfRecv.Deobfuscate(enc)
	if err == nil {
		t.Error("Corrupted packet should not decode successfully")
	}
}

func TestChaCha20Obfuscator_RandomInput(t *testing.T) {
	buf := make([]byte, 120)
	_, _ = rand.Read(buf)
	_ = &unstrictMarshaller{Data: make([]byte, 32)}
	obf := newTestObfuscator[*unstrictMarshaller](30, 200)
	_, err := obf.Deobfuscate(buf)
	if err == nil {
		t.Error("Garbage input should not decode successfully")
	}
}

func TestChaCha20Obfuscator_HandshakeBiggerThanMaxLen(t *testing.T) {
	orig := &unstrictMarshaller{Data: make([]byte, 300)}
	_, _ = rand.Read(orig.Data)
	obf := newTestObfuscator[*unstrictMarshaller](90, 100)
	enc, err := obf.Obfuscate(orig)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	if len(enc) < 300 {
		t.Errorf("Packet must be at least handshake size: got %d", len(enc))
	}
	_ = &unstrictMarshaller{Data: make([]byte, 300)}
	obfRecv := newTestObfuscator[*unstrictMarshaller](90, 100)
	got, err := obfRecv.Deobfuscate(enc)
	if err != nil {
		t.Fatalf("Deobfuscate: %v", err)
	}
	if !bytes.Equal(orig.Data, got.Data) {
		t.Error("Data mismatch on long handshake")
	}
}

func TestChaCha20Obfuscator_MinEqualsMax(t *testing.T) {
	orig := &unstrictMarshaller{Data: make([]byte, 24)}
	_, _ = rand.Read(orig.Data)
	obf := newTestObfuscator[*unstrictMarshaller](64, 64)
	enc, err := obf.Obfuscate(orig)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	if len(enc) < 24 {
		t.Error("Packet shorter than handshake data")
	}
	enc2, _ := obf.Obfuscate(orig)
	if len(enc) != len(enc2) {
		t.Error("Packet length should be same when min==max")
	}
}

func TestChaCha20Obfuscator_MarshalBinaryFail(t *testing.T) {
	mar := &failingMarshaller{}
	obf := newTestObfuscator[*failingMarshaller](32, 100)
	_, err := obf.Obfuscate(mar)
	if err == nil {
		t.Error("MarshalBinary error must propagate")
	}
}

func TestChaCha20Obfuscator_UnmarshalBinaryFail(t *testing.T) {
	orig := &unstrictMarshaller{Data: make([]byte, 16)}
	_, _ = rand.Read(orig.Data)
	obf := newTestObfuscator[*unstrictMarshaller](20, 50)
	enc, err := obf.Obfuscate(orig)
	if err != nil {
		t.Fatalf("Obfuscate: %v", err)
	}
	// Use a marshaller which always fails on Unmarshal
	_ = &failingMarshaller{}
	obfRecv := newTestObfuscator[*failingMarshaller](20, 50)
	_, err = obfRecv.Deobfuscate(enc)
	if err == nil {
		t.Error("UnmarshalBinary error must propagate")
	}
}

func TestChaCha20Obfuscator_LengthPaddingAndRandomization(t *testing.T) {
	orig := &unstrictMarshaller{Data: make([]byte, 8)}
	_, _ = rand.Read(orig.Data)
	obf := newTestObfuscator[*unstrictMarshaller](64, 256)
	foundDifferent := false
	var prevLen int
	for i := 0; i < 20; i++ {
		enc, err := obf.Obfuscate(orig)
		if err != nil {
			t.Fatalf("Obfuscate: %v", err)
		}
		if i > 0 && prevLen != len(enc) {
			foundDifferent = true
		}
		prevLen = len(enc)
	}
	if !foundDifferent {
		t.Error("Packet length is not randomized")
	}
}
