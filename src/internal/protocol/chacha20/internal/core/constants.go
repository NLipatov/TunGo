package core

import "golang.org/x/crypto/chacha20poly1305"

const (
	SessionIdentifierLength = 32
	DirectionLength         = 16
	AADLength               = SessionIdentifierLength + chacha20poly1305.NonceSize + DirectionLength
	// MaxEpoch is the highest epoch that may be allocated for one session.
	MaxEpoch uint16 = 65000
)
