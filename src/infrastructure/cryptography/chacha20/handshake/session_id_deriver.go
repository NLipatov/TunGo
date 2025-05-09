package handshake

import (
	"fmt"
	"io"
)

type SessionIdentifier interface {
	Identify() ([32]byte, error)
}

type DefaultSessionIdentifier struct {
	reader io.Reader
}

func NewSessionIdentifier(reader io.Reader) SessionIdentifier {
	return &DefaultSessionIdentifier{
		reader: reader,
	}
}

func (s *DefaultSessionIdentifier) Identify() ([32]byte, error) {
	var sessionID [32]byte

	if _, err := io.ReadFull(s.reader, sessionID[:]); err != nil {
		return [32]byte{}, fmt.Errorf("failed to derive session ID: %w", err)
	}

	return sessionID, nil
}
