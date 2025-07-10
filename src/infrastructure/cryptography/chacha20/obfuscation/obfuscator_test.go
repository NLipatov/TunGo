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
	if len(b) != len(d.Data) {
		return errors.New("wrong length")
	}
	copy(d.Data, b)
	return nil
}

func newTestObfuscator(bufLen int) (*ChaCha20Obfuscator[*unstrictMarshaller], *unstrictMarshaller) {
	key := sha256.Sum256([]byte("test-key"))
	psk := []byte("psk-test-xyz123456789")
	hmacKey := sha256.Sum256([]byte("test-hmac-key"))
	hmacImpl := hmac.NewHMAC(hmacKey[:])
	plain := &unstrictMarshaller{Data: make([]byte, bufLen)}
	_, _ = rand.Read(plain.Data)
	obf := NewChaCha20Obfuscator(plain, key[:], psk, hmacImpl, chacha20.NewSliding64())
	return &obf, plain
}

func TestChaCha20Obfuscator_Roundtrip(t *testing.T) {
	obf, orig := newTestObfuscator(64)
	enc, err := obf.MarshalObfuscatedBinary()
	if err != nil {
		t.Fatalf("MarshalObfuscatedBinary: %v", err)
	}

	recv := &unstrictMarshaller{Data: make([]byte, 64)}
	obfRecv := NewChaCha20Obfuscator(recv, obf.key, obf.psk, obf.hmac, chacha20.NewSliding64())
	if err := obfRecv.UnmarshalObfuscatedBinary(enc); err != nil {
		t.Fatalf("UnmarshalObfuscatedBinary: %v", err)
	}
	if !bytes.Equal(orig.Data, recv.Data) {
		t.Error("Obfuscator did not roundtrip the data")
	}
}

func TestChaCha20Obfuscator_Corrupted(t *testing.T) {
	obf, _ := newTestObfuscator(32)
	enc, err := obf.MarshalObfuscatedBinary()
	if err != nil {
		t.Fatalf("MarshalObfuscatedBinary: %v", err)
	}
	for i := 10; i < 20 && i < len(enc); i++ {
		enc[i] ^= 0x77
	}
	recv := &strictMarshaller{Data: make([]byte, 32)}
	obfRecv := NewChaCha20Obfuscator(recv, obf.key, obf.psk, obf.hmac, chacha20.NewSliding64())
	if err := obfRecv.UnmarshalObfuscatedBinary(enc); err == nil {
		t.Error("Corrupted packet should not decode successfully")
	}
}

func TestChaCha20Obfuscator_InvalidInput(t *testing.T) {
	buf := make([]byte, 100)
	_, _ = rand.Read(buf)
	recv := &unstrictMarshaller{Data: make([]byte, 32)}
	key := sha256.Sum256([]byte("test-key"))
	psk := []byte("psk-test-xyz123456789")
	hmacKey := sha256.Sum256([]byte("test-hmac-key"))
	hmacImpl := hmac.NewHMAC(hmacKey[:])
	obfRecv := NewChaCha20Obfuscator(recv, key[:], psk, hmacImpl, chacha20.NewSliding64())
	if err := obfRecv.UnmarshalObfuscatedBinary(buf); err == nil {
		t.Error("Garbage input should not decode successfully")
	}
}

func TestChaCha20Obfuscator_PlainGetter(t *testing.T) {
	obf, orig := newTestObfuscator(8)
	if obf.Plain() != orig {
		t.Error("Plain getter did not return the original instance")
	}
}
