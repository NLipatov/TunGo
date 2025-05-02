package handshake

import (
	"bytes"
	"crypto/sha256"
	"golang.org/x/crypto/hkdf"
	"io"
	"testing"
)

func TestIdentify_SameOutputOnRepeatedCalls(t *testing.T) {
	secret := []byte("some-secret-value")
	salt := []byte("some-salt-value")

	ident := NewSessionIdentifier(secret, salt)

	// первый вызов
	id1, err := ident.Identify()
	if err != nil {
		t.Fatalf("Identify() failed: %v", err)
	}
	// второй вызов
	id2, err := ident.Identify()
	if err != nil {
		t.Fatalf("second Identify() failed: %v", err)
	}

	if !bytes.Equal(id1[:], id2[:]) {
		t.Errorf("Identify() not deterministic: first=%x second=%x", id1, id2)
	}
	if len(id1) != 32 {
		t.Errorf("expected 32‑byte session ID, got %d bytes", len(id1))
	}
}

func TestIdentify_DifferentSecretOrSalt(t *testing.T) {
	secret1 := []byte("secret1")
	secret2 := []byte("secret2")
	salt1 := []byte("salt1")
	salt2 := []byte("salt2")

	idA, _ := NewSessionIdentifier(secret1, salt1).Identify()
	idB, _ := NewSessionIdentifier(secret1, salt2).Identify()
	if bytes.Equal(idA[:], idB[:]) {
		t.Errorf("IDs should differ when salt differs: %x vs %x", idA, idB)
	}

	idC, _ := NewSessionIdentifier(secret2, salt1).Identify()
	if bytes.Equal(idA[:], idC[:]) {
		t.Errorf("IDs should differ when secret differs: %x vs %x", idA, idC)
	}
}

func TestIdentify_MatchesDirectHKDF(t *testing.T) {
	secret := []byte("another-secret")
	salt := []byte("another-salt")

	// вручную через hkdf
	var expected [32]byte
	h := hkdf.New(sha256.New, secret, salt, []byte("session-id-derivation"))
	if _, err := io.ReadFull(h, expected[:]); err != nil {
		t.Fatalf("manual HKDF read failed: %v", err)
	}

	got, err := NewSessionIdentifier(secret, salt).Identify()
	if err != nil {
		t.Fatalf("Identify() error: %v", err)
	}
	if !bytes.Equal(got[:], expected[:]) {
		t.Errorf("Identify() = %x; want %x", got, expected)
	}
}
