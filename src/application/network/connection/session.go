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

// SessionAuth provides AllowedIPs enforcement functionality.
// SECURITY INVARIANT: All sessions MUST implement this interface.
// IsSourceAllowed MUST be called for every ingress packet after decryption.
type SessionAuth interface {
	// IsSourceAllowed checks if the given source IP is allowed for this session.
	// Returns true if srcIP equals the internal IP or is within any allowed prefix.
	// MUST normalize IPv4-mapped-IPv6 addresses before comparison.
	IsSourceAllowed(srcIP netip.Addr) bool
}

// Session is the complete interface for established secure sessions.
// Embeds SessionAuth to enforce AllowedIPs checking at the type level.
type Session interface {
	SessionMeta
	SessionCrypto
	SessionRekey
	SessionAuth
}

// SessionIdentity provides access to the client's cryptographic identity.
// Optional interface for sessions created with authentication info.
type SessionIdentity interface {
	// ClientPubKey returns the client's X25519 static public key.
	// May return nil for sessions without authentication info.
	ClientPubKey() []byte
}
