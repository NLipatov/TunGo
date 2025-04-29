package handshake

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
)

func TestGenerateEd25519Keys_RoundTrip(t *testing.T) {
	crypto := NewDefaultClientCrypto()
	pub, priv, err := crypto.GenerateEd25519Keys()
	if err != nil {
		t.Fatalf("GenerateEd25519Keys error: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Errorf("public key size = %d; want %d", len(pub), ed25519.PublicKeySize)
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Errorf("private key size = %d; want %d", len(priv), ed25519.PrivateKeySize)
	}
	data := []byte("hello")
	sig := crypto.Sign(priv, data)
	if !crypto.Verify(pub, data, sig) {
		t.Errorf("Verify failed for valid signature")
	}
	if crypto.Verify(pub, []byte("bad"), sig) {
		t.Errorf("Verify succeeded for invalid data")
	}
}

func TestNewX25519SessionKeyPair_Success(t *testing.T) {
	crypto := NewDefaultClientCrypto()
	pub, priv, err := crypto.NewX25519SessionKeyPair()
	if err != nil {
		t.Fatalf("NewX25519SessionKeyPair error: %v", err)
	}
	if len(pub) != 32 {
		t.Errorf("curve public size = %d; want 32", len(pub))
	}
	if len(priv) != curve25519.ScalarSize {
		t.Errorf("curve private size = %d; want %d", len(priv), curve25519.ScalarSize)
	}
}

func TestNewX25519SessionKeyPair_Error(t *testing.T) {
	// force ReadFull to fail by swapping rand.Reader
	old := rand.Reader
	rand.Reader = &badReader{}
	defer func() { rand.Reader = old }()

	crypto := NewDefaultClientCrypto()
	_, _, err := crypto.NewX25519SessionKeyPair()
	if err == nil {
		t.Errorf("expected generation error")
	}
}

// badReader always returns error
type badReader struct{}

func (badReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("could not read data")
}

func TestGenerateSalt_Uniqueness(t *testing.T) {
	crypto := NewDefaultClientCrypto()
	s1 := crypto.GenerateSalt()
	s2 := crypto.GenerateSalt()
	if len(s1) != 32 {
		t.Errorf("salt len = %d; want 32", len(s1))
	}
	if bytes.Equal(s1, s2) {
		t.Errorf("salts should differ")
	}
}

func TestCalculateKeys_Consistency(t *testing.T) {
	crypto := NewDefaultClientCrypto()
	// generate key pair for this side
	_, sessionPriv, _ := crypto.NewX25519SessionKeyPair()
	// generate peer public key
	serverPub, _, _ := crypto.NewX25519SessionKeyPair()
	salt := crypto.GenerateSalt()
	nonce := crypto.GenerateSalt()[:]

	s2c, c2s, sid, err := crypto.CalculateKeys(sessionPriv[:], salt, nonce, serverPub)
	if err != nil {
		t.Fatalf("CalculateKeys error: %v", err)
	}
	if len(s2c) != chacha20poly1305.KeySize {
		t.Errorf("server-to-client len = %d; want %d", len(s2c), chacha20poly1305.KeySize)
	}
	if len(c2s) != chacha20poly1305.KeySize {
		t.Errorf("client-to-server len = %d; want %d", len(c2s), chacha20poly1305.KeySize)
	}

	// compute shared secret exactly as in CalculateKeys
	shared, _ := curve25519.X25519(sessionPriv[:], serverPub)
	// saltHash = SHA256(nonce || salt)
	hash := sha256.Sum256(append(nonce, salt...))
	expected, _ := deriveSessionId(shared, hash[:])
	if sid != expected {
		t.Errorf("sessionID mismatch: got %x, want %x", sid, expected)
	}
}
