package handshake

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

type crypto interface {
	// deriveTwoKeys reproducibly derives two AEAD keys from sharedSecret,
	// extNonce (e.g. HKDF salt-nonce) and sessionSalt.
	deriveTwoKeys(
		sharedSecret []byte,
		extNonce []byte,
		sessionSalt []byte,
	) (serverToClientKey, clientToServerKey []byte, err error)
}

type defaultCrypto struct {
}

func NewDefaultCrypto() crypto {
	return &defaultCrypto{}
}

// DeriveTwoKeys reproducibly derives two AEAD keys from sharedSecret,
// extNonce (e.g. HKDF salt-nonce) and sessionSalt.
func (d *defaultCrypto) deriveTwoKeys(
	sharedSecret []byte,
	extNonce []byte,
	sessionSalt []byte,
) (serverToClientKey, clientToServerKey []byte, err error) {
	// HKDF salt = SHA256(extNonce || sessionSalt)
	hash := sha256.Sum256(append(extNonce, sessionSalt...))

	infoSC := []byte("server-to-client")
	infoCS := []byte("client-to-server")

	hkdfSC := hkdf.New(sha256.New, sharedSecret, hash[:], infoSC)
	hkdfCS := hkdf.New(sha256.New, sharedSecret, hash[:], infoCS)

	keySize := chacha20poly1305.KeySize

	serverToClientKey = make([]byte, keySize)
	if _, err = io.ReadFull(hkdfSC, serverToClientKey); err != nil {
		err = fmt.Errorf("HKDF server→client: %w", err)
		return
	}

	clientToServerKey = make([]byte, keySize)
	if _, err = io.ReadFull(hkdfCS, clientToServerKey); err != nil {
		err = fmt.Errorf("HKDF client→server: %w", err)
		return
	}

	return
}
