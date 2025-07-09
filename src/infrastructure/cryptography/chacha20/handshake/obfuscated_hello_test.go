package handshake

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"
	"tungo/infrastructure/cryptography/hmac"
)

var corruptionMagic = []byte{42, 111, 23, 7}

type strictHelloStub struct {
	Data []byte
}

func (h *strictHelloStub) Nonce() []byte {
	panic("not implemented")
}

func (h *strictHelloStub) CurvePublicKey() []byte {
	panic("not implemented")
}

func (h *strictHelloStub) MarshalBinary() ([]byte, error) {
	return append(append([]byte{}, corruptionMagic...), h.Data...), nil
}

func (h *strictHelloStub) UnmarshalBinary(b []byte) error {
	if len(b) != len(h.Data)+len(corruptionMagic) {
		return errors.New("wrong length")
	}
	if !bytes.Equal(b[:len(corruptionMagic)], corruptionMagic) {
		return errors.New("bad magic")
	}
	copy(h.Data, b[len(corruptionMagic):])
	return nil
}

type obfHelloStub struct {
	Data []byte
}

func (h *obfHelloStub) Nonce() []byte                  { return h.Data[:12] }
func (h *obfHelloStub) CurvePublicKey() []byte         { return h.Data[12:44] }
func (h *obfHelloStub) MarshalBinary() ([]byte, error) { return h.Data, nil }
func (h *obfHelloStub) UnmarshalBinary(b []byte) error {
	if len(b) != len(h.Data) {
		return errors.New("wrong length")
	}
	copy(h.Data, b)
	return nil
}

func TestFloatingObfuscatedClientHello_Roundtrip(t *testing.T) {
	plain := &obfHelloStub{Data: make([]byte, 64)}
	_, readErr := rand.Read(plain.Data)
	if readErr != nil {
		t.Fatal(readErr)
	}

	key := sha256.Sum256([]byte("encryption-key"))
	psk := []byte("psk-12345678901234567890")
	hmacKey := sha256.Sum256([]byte("hmac-key-xyz"))
	hmacImpl := hmac.NewHMAC(hmacKey[:])

	hello := NewFloatingObfuscatedClientHello(plain, key[:], psk, hmacImpl)

	packet, err := hello.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	recvPlain := &obfHelloStub{Data: make([]byte, 64)}
	recv := NewFloatingObfuscatedClientHello(recvPlain, key[:], psk, hmacImpl)
	if err := recv.UnmarshalBinary(packet); err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}
	decoded := recv.(*ObfuscatedHello).hello.(*obfHelloStub)
	if !bytes.Equal(plain.Data, decoded.Data) {
		t.Errorf("Decoded hello does not match original")
	}
}

func TestFloatingObfuscatedClientHello_Corrupt(t *testing.T) {
	plain := &strictHelloStub{Data: make([]byte, 64)}
	_, readErr := rand.Read(plain.Data)
	if readErr != nil {
		t.Fatal(readErr)
	}

	key := sha256.Sum256([]byte("encryption-key"))
	psk := []byte("psk-98765432109876543210")
	hmacKey := sha256.Sum256([]byte("hmac-key-abc"))
	hmacImpl := hmac.NewHMAC(hmacKey[:])

	hello := NewFloatingObfuscatedClientHello(plain, key[:], psk, hmacImpl)
	packet, err := hello.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	// Damage multiple bytes to avoid lucky decrypt
	for i := 10; i < 15 && i < len(packet); i++ {
		packet[i] ^= 0xFF
	}
	recv := NewFloatingObfuscatedClientHello(&obfHelloStub{Data: make([]byte, 64)}, key[:], psk, hmacImpl)
	if err := recv.UnmarshalBinary(packet); err == nil {
		t.Error("Corrupted packet should not decode")
	}
}

func TestFloatingObfuscatedClientHello_OffsetsDiffer(t *testing.T) {
	plain := &obfHelloStub{Data: make([]byte, 64)}
	_, readErr := rand.Read(plain.Data)
	if readErr != nil {
		t.Fatal(readErr)
	}

	key := sha256.Sum256([]byte("enc-key-abc"))
	psk := []byte("psk-zxcvbnmlkjhgfdsapoiu")
	hmacKey := sha256.Sum256([]byte("hmac-key-off-diff"))
	hmacImpl := hmac.NewHMAC(hmacKey[:])

	offsets := make(map[int]struct{})
	for i := 0; i < 20; i++ {
		hello := NewFloatingObfuscatedClientHello(plain, key[:], psk, hmacImpl)
		packet, err := hello.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary failed: %v", err)
		}
		found := false
		for offset := 0; offset <= len(packet)-nonceLen-2; offset++ {
			marker := make([]byte, 2)
			copy(marker, packet[offset:offset+2])
			nonce := make([]byte, nonceLen)
			copy(nonce, packet[offset+2:offset+2+nonceLen])
			hmacInput := append([]byte{}, psk...)
			hmacInput = append(hmacInput, nonce...)
			expectedMarkerFull, _ := hmacImpl.Generate(hmacInput)
			expectedMarker := make([]byte, 2)
			copy(expectedMarker, expectedMarkerFull[:2])
			if bytes.Equal(marker, expectedMarker) {
				offsets[offset] = struct{}{}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Cannot find marker in packet")
		}
	}
	if len(offsets) < 5 {
		t.Error("Handshakes should appear at different offsets in packets")
	}
}
