package handshake

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

type Crypto interface {
	Sign(privateKey ed25519.PrivateKey, data []byte) []byte
	Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
	GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error)
	GenerateX25519KeyPair() ([]byte, [32]byte, error)
	GenerateRandomBytesArray(size int) []byte
	GenerateChaCha20KeysServerside(
		curvePrivate, serverNonce []byte,
		hello Hello) (sessionId [32]byte, clientToServerKey, serverToClientKey []byte, err error)
	GenerateChaCha20KeysClientside(curvePrivate, sessionSalt []byte,
		hello Hello) ([]byte, []byte, [32]byte, error)
	DeriveKey(
		sharedSecret []byte,
		salt []byte,
		info []byte,
	) ([]byte, error)
}

type DefaultCrypto struct {
}

func newDefaultCrypto() Crypto {
	return &DefaultCrypto{}
}

func (d *DefaultCrypto) Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool {
	return ed25519.Verify(publicKey, data, signature)
}

func (d *DefaultCrypto) Sign(privateKey ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(privateKey, data)
}
func (d *DefaultCrypto) GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (d *DefaultCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	var private [32]byte

	_, privateErr := io.ReadFull(rand.Reader, private[:])
	if privateErr != nil {
		return nil, private, privateErr
	}

	public, publicErr := curve25519.X25519(private[:], curve25519.Basepoint)

	return public, private, publicErr
}

func (d *DefaultCrypto) GenerateRandomBytesArray(size int) []byte {
	randomSalt := make([]byte, size)
	_, _ = io.ReadFull(rand.Reader, randomSalt)
	return randomSalt
}
func (d *DefaultCrypto) GenerateChaCha20KeysServerside(
	curvePrivate,
	serverNonce []byte,
	hello Hello) (sessionId [32]byte, clientToServerKey, serverToClientKey []byte, err error) {
	// Generate shared secret and salt
	sharedSecret, _ := curve25519.X25519(curvePrivate[:], hello.CurvePublicKey())
	salt := sha256.Sum256(append(serverNonce, hello.Nonce()...))

	serverToClientKey, err = d.DeriveKey(
		sharedSecret,
		salt[:],
		[]byte("server-to-client"),
	)
	if err != nil {
		return [32]byte{}, nil, nil, err
	}

	clientToServerKey, err = d.DeriveKey(
		sharedSecret,
		salt[:],
		[]byte("client-to-server"),
	)
	if err != nil {
		return [32]byte{}, nil, nil, err
	}

	readerFactory := NewDefaultSessionIdReader([]byte("session-id-derivation"), sharedSecret, salt[:])
	identifier := NewSessionIdentifier(readerFactory.NewReader())
	sessionId, sessionIdErr := identifier.Identify()
	if sessionIdErr != nil {
		return [32]byte{},
			nil,
			nil,
			fmt.Errorf("failed to derive session id: %s", sessionId)
	}

	return sessionId, clientToServerKey, serverToClientKey, nil
}
func (d *DefaultCrypto) GenerateChaCha20KeysClientside(curvePrivate, clientNonce []byte, hello Hello) ([]byte, []byte, [32]byte, error) {
	sharedSecret, _ := curve25519.X25519(curvePrivate[:], hello.CurvePublicKey())
	salt := sha256.Sum256(append(hello.Nonce(), clientNonce...))
	serverToClientKey, err := d.DeriveKey(
		sharedSecret,
		salt[:],
		[]byte("server-to-client"),
	)
	if err != nil {
		return nil, nil, [32]byte{}, err
	}

	clientToServerKey, err := d.DeriveKey(
		sharedSecret,
		salt[:],
		[]byte("client-to-server"),
	)
	if err != nil {
		return nil, nil, [32]byte{}, err
	}

	readerFactory := NewDefaultSessionIdReader([]byte("session-id-derivation"), sharedSecret, salt[:])
	identifier := NewSessionIdentifier(readerFactory.NewReader())
	sessionId, sessionIdErr := identifier.Identify()
	if sessionIdErr != nil {
		return nil, nil, [32]byte{}, fmt.Errorf("failed to derive session id: %w", sessionIdErr)
	}

	return serverToClientKey, clientToServerKey, sessionId, nil
}

func (d *DefaultCrypto) DeriveKey(
	sharedSecret []byte,
	salt []byte,
	info []byte,
) ([]byte, error) {
	hkdfReader := hkdf.New(
		sha256.New,
		sharedSecret,
		salt,
		info,
	)
	key := make([]byte, chacha20poly1305.KeySize)
	_, err := io.ReadFull(hkdfReader, key)
	return key, err
}
