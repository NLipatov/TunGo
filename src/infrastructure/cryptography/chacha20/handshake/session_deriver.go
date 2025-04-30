package handshake

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

type SessionIdDeriver interface {
	Derive() ([32]byte, error)
}

type DefaultSessionIdDeriver struct {
	sharedSecret, salt []byte
}

func NewDefaultSessionIdDeriver(sharedSecret, salt []byte) SessionIdDeriver {
	return &DefaultSessionIdDeriver{
		sharedSecret: sharedSecret,
		salt:         salt,
	}
}

func (d *DefaultSessionIdDeriver) Derive() ([32]byte, error) {
	var sessionID [32]byte

	hkdfReader := hkdf.New(sha256.New, d.sharedSecret, d.salt, []byte("session-id-derivation"))
	if _, err := io.ReadFull(hkdfReader, sessionID[:]); err != nil {
		return [32]byte{}, fmt.Errorf("failed to derive session ID: %w", err)
	}

	return sessionID, nil
}
