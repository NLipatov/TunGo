package handshake

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

// stub Hello compliant with the Hello interface
type helloStub struct {
	curvePub, nonce []byte
}

func (h *helloStub) CurvePublicKey() []byte { return h.curvePub }
func (h *helloStub) Nonce() []byte          { return h.nonce }

func TestSignVerify(t *testing.T) {
	c := newDefaultCrypto()

	pub, priv, err := c.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}
	msg := []byte("test message")
	sig := c.Sign(priv, msg)
	if !c.Verify(pub, msg, sig) {
		t.Error("Verify should succeed for valid signature")
	}

	newData := []byte("bad")
	if c.Verify(pub, newData, sig) {
		t.Error("Verify should fail on wrong message")
	}

	newPubKey, _, _ := c.GenerateEd25519KeyPair()
	if c.Verify(newPubKey, msg, sig) {
		t.Error("Verify should fail on wrong public key")
	}
}

func TestGenerateEd25519KeyPairSizes(t *testing.T) {
	c := newDefaultCrypto()
	pub, priv, err := c.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Errorf("pub size = %d; want %d", len(pub), ed25519.PublicKeySize)
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Errorf("priv size = %d; want %d", len(priv), ed25519.PrivateKeySize)
	}
}

func TestGenerateX25519KeyPair_SymmetricSecret(t *testing.T) {
	c := newDefaultCrypto()
	srvPub, srvPriv, err := c.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair: %v", err)
	}
	clPub, clPriv, err := c.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair: %v", err)
	}
	// shared from both sides must match
	ss1, err := curve25519.X25519(srvPriv[:], clPub)
	if err != nil {
		t.Fatal(err)
	}
	ss2, err := curve25519.X25519(clPriv[:], srvPub)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ss1, ss2) {
		t.Error("shared secrets differ")
	}
}

func TestGenerateRandomBytesArray(t *testing.T) {
	c := newDefaultCrypto()
	a := c.GenerateRandomBytesArray(16)
	b := c.GenerateRandomBytesArray(16)
	if len(a) != 16 || len(b) != 16 {
		t.Errorf("expected length=16; got %d, %d", len(a), len(b))
	}
	if bytes.Equal(a, b) {
		t.Error("expected different random slices")
	}
}

func TestGenerateChaCha20Keys_BothSidesMatch(t *testing.T) {
	c := newDefaultCrypto()

	// simulate key exchange
	srvPub, srvPriv, err := c.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clPub, clPriv, err := c.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}

	// nonces
	srvNonce := c.GenerateRandomBytesArray(nonceLength)
	clNonce := c.GenerateRandomBytesArray(nonceLength)

	// server side derive
	hCl := &helloStub{curvePub: clPub, nonce: clNonce}
	sID1, c2s1, s2c1, err := c.GenerateChaCha20KeysServerside(srvPriv[:], srvNonce, hCl)
	if err != nil {
		t.Fatalf("Serverside derivation: %v", err)
	}
	// client side derive
	hSrv := &helloStub{curvePub: srvPub, nonce: srvNonce}
	s2c2, c2s2, sID2, err := c.GenerateChaCha20KeysClientside(clPriv[:], clNonce, hSrv)
	if err != nil {
		t.Fatalf("Clientside derivation: %v", err)
	}

	// keys must match across client/server
	if !bytes.Equal(s2c1, s2c2) {
		t.Error("server→client keys differ")
	}
	if !bytes.Equal(c2s1, c2s2) {
		t.Error("client→server keys differ")
	}
	// session IDs must match
	if sID1 != sID2 {
		t.Error("session IDs differ")
	}
	// length checks
	if len(s2c1) != chacha20poly1305.KeySize || len(c2s1) != chacha20poly1305.KeySize {
		t.Errorf("unexpected key size: %d, %d", len(s2c1), len(c2s1))
	}
}
