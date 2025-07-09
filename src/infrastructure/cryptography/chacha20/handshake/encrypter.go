package handshake

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"fmt"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/nacl/box"
)

// Encrypter encrypts and decrypts handshake messages using Curve25519
// public-key encryption. If keys are nil, encryption/decryption is skipped.
type Encrypter struct {
	public  *[32]byte
	private *[32]byte
}

// NewEncrypter constructs an Encrypter with the given keys.
func NewEncrypter(pub, priv *[32]byte) Encrypter {
	return Encrypter{public: pub, private: priv}
}

// Encrypt encrypts data with the configured public key. If no public key is
// configured, the original data is returned.
func (e Encrypter) Encrypt(data []byte) ([]byte, error) {
	if e.public == nil {
		return data, nil
	}
	out, err := box.SealAnonymous(nil, data, e.public, rand.Reader)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Decrypt decrypts data using the configured key pair. The returned boolean
// indicates whether decryption was performed.
func (e Encrypter) Decrypt(data []byte) ([]byte, bool, error) {
	if e.public == nil || e.private == nil {
		return data, false, nil
	}
	out, ok := box.OpenAnonymous(nil, data, e.public, e.private)
	if !ok {
		return nil, false, fmt.Errorf("decryption failed")
	}
	return out, true, nil
}

// ed25519PrivateKeyToCurve25519 converts an Ed25519 private key to a Curve25519
// private key as specified by RFC 7748.
func ed25519PrivateKeyToCurve25519(priv ed25519.PrivateKey) [32]byte {
	seed := priv.Seed()
	h := sha512.Sum512(seed)
	h[0] &^= 7
	h[31] &^= 0x80
	h[31] |= 0x40
	var out [32]byte
	copy(out[:], h[:32])
	return out
}

// ed25519PublicKeyToCurve25519 converts an Ed25519 public key to a Curve25519
// public key using the birational map between the curves.
func ed25519PublicKeyToCurve25519(pub ed25519.PublicKey) ([32]byte, error) {
	point, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil {
		return [32]byte{}, err
	}
	outBytes := point.BytesMontgomery()
	var out [32]byte
	copy(out[:], outBytes)
	return out, nil
}
