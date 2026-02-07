package primitives

import (
	crand "crypto/rand"
	"errors"
	"io"
	"testing"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy read failed")
}

func TestDefaultKeyDeriver_GenerateX25519KeyPair_Success(t *testing.T) {
	d := &DefaultKeyDeriver{}

	pub, priv, err := d.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub) != 32 {
		t.Fatalf("expected public key length 32, got %d", len(pub))
	}
	if len(priv) != 32 {
		t.Fatalf("expected private key length 32, got %d", len(priv))
	}
}

func TestDefaultKeyDeriver_GenerateX25519KeyPair_ReadError(t *testing.T) {
	orig := crand.Reader
	crand.Reader = io.Reader(errReader{})
	t.Cleanup(func() {
		crand.Reader = orig
	})

	d := &DefaultKeyDeriver{}
	_, _, err := d.GenerateX25519KeyPair()
	if err == nil {
		t.Fatal("expected entropy read error")
	}
}

func TestDefaultKeyDeriver_DeriveKey(t *testing.T) {
	d := &DefaultKeyDeriver{}

	sharedSecret := []byte("shared-secret")
	salt := []byte("salt")
	info1 := []byte("ctx-1")
	info2 := []byte("ctx-2")

	key1a, err := d.DeriveKey(sharedSecret, salt, info1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	key1b, err := d.DeriveKey(sharedSecret, salt, info1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	key2, err := d.DeriveKey(sharedSecret, salt, info2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(key1a) != 32 {
		t.Fatalf("expected derived key length 32, got %d", len(key1a))
	}
	if string(key1a) != string(key1b) {
		t.Fatal("expected deterministic output for same inputs")
	}
	if string(key1a) == string(key2) {
		t.Fatal("expected different output for different info context")
	}
}
