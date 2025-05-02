package handshake

import (
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/hkdf"
	"io"
)

type SessionIdentifier interface {
	Identify() ([32]byte, error)
}

type DefaultSessionIdentifier struct {
	secret, salt []byte
}

func NewSessionIdentifier(sharedSecret, salt []byte) SessionIdentifier {
	return &DefaultSessionIdentifier{
		secret: sharedSecret,
		salt:   salt,
	}
}

func (s *DefaultSessionIdentifier) Identify() ([32]byte, error) {
	var sessionID [32]byte

	hkdfReader := hkdf.New(sha256.New, s.secret, s.salt, []byte("session-id-derivation"))
	if _, err := io.ReadFull(hkdfReader, sessionID[:]); err != nil {
		return [32]byte{}, fmt.Errorf("failed to derive session ID: %w", err)
	}

	return sessionID, nil
}
