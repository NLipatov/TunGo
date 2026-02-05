package session

import (
	"net/netip"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// Session represents a single encrypted session between a VPN client and server.
type Session struct {
	crypto     connection.Crypto
	fsm        rekey.FSM
	internalIP netip.Addr
	externalIP netip.AddrPort

	// clientPubKey is the client's X25519 static public key (cryptographic identity).
	clientPubKey []byte

	// allowedIPs are the additional prefixes this client may use as source IP.
	// The internalIP is always implicitly allowed.
	allowedIPs []netip.Prefix
}

func NewSession(
	crypto connection.Crypto,
	fsm rekey.FSM,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
) connection.Session {
	return &Session{
		crypto:     crypto,
		fsm:        fsm,
		internalIP: internalIP,
		externalIP: externalIP,
	}
}

// NewSessionWithAuth creates a session with client authentication info.
func NewSessionWithAuth(
	crypto connection.Crypto,
	fsm rekey.FSM,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	clientPubKey []byte,
	allowedIPs []netip.Prefix,
) *Session {
	return &Session{
		crypto:       crypto,
		fsm:          fsm,
		internalIP:   internalIP,
		externalIP:   externalIP,
		clientPubKey: clientPubKey,
		allowedIPs:   allowedIPs,
	}
}

func (s *Session) InternalAddr() netip.Addr {
	return s.internalIP
}

func (s *Session) ExternalAddrPort() netip.AddrPort {
	return s.externalIP
}

func (s *Session) Crypto() connection.Crypto {
	return s.crypto
}

func (s *Session) RekeyController() rekey.FSM {
	return s.fsm
}

// ClientPubKey returns the client's X25519 static public key.
func (s *Session) ClientPubKey() []byte {
	return s.clientPubKey
}

// AllowedIPs returns the additional prefixes this client may use as source IP.
func (s *Session) AllowedIPs() []netip.Prefix {
	return s.allowedIPs
}

// IsSourceAllowed checks if the given source IP is allowed for this session.
// Returns true if srcIP equals the internal IP or is within any allowed prefix.
// Normalizes IPv4-mapped-IPv6 addresses (e.g., ::ffff:10.0.0.5 → 10.0.0.5) before comparison.
func (s *Session) IsSourceAllowed(srcIP netip.Addr) bool {
	// Normalize IPv4-mapped-IPv6 to pure IPv4 for consistent comparison
	normalizedSrc := srcIP.Unmap()
	normalizedInternal := s.internalIP.Unmap()

	// Internal IP is always allowed
	if normalizedSrc == normalizedInternal {
		return true
	}
	// Check additional AllowedIPs
	for _, prefix := range s.allowedIPs {
		// Unmap prefix address for consistent matching
		prefixAddr := prefix.Addr().Unmap()
		normalizedPrefix := netip.PrefixFrom(prefixAddr, prefix.Bits())
		if normalizedPrefix.Contains(normalizedSrc) {
			return true
		}
	}
	return false
}
