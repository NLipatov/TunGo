package connection

import (
	"net/netip"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// Session is abstraction over established secure session of data-transfer between 2 hosts.
type SessionMeta interface {
	// ExternalAddrPort returns the external (outside VPN) address of the client.
	// Multiple clients may share the same external IP address (e.g., behind NAT).
	ExternalAddrPort() netip.AddrPort

	// InternalAddr returns the internal (inside VPN) IP address of the client.
	// Each client has a unique internal address in the virtual private network.
	InternalAddr() netip.Addr
}

type SessionCrypto interface {
	// Crypto is a getter for Crypto, which used for encryption/decryption operations.
	Crypto() Crypto
}

type SessionRekey interface {
	// RekeyController returns control-plane rekey state; may be nil for protocols without rekey.
	RekeyController() rekey.FSM
}

type Session interface {
	SessionMeta
	SessionCrypto
	SessionRekey
}
