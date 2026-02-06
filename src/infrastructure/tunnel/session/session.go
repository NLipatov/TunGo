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
// AllowedIPs prefixes are normalized (Unmap) at creation time to avoid
// per-packet allocations in IsSourceAllowed.
func NewSessionWithAuth(
	crypto connection.Crypto,
	fsm rekey.FSM,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	clientPubKey []byte,
	allowedIPs []netip.Prefix,
) *Session {
	normalized := make([]netip.Prefix, len(allowedIPs))
	for i, p := range allowedIPs {
		normalized[i] = netip.PrefixFrom(p.Addr().Unmap(), p.Bits())
	}
	return &Session{
		crypto:       crypto,
		fsm:          fsm,
		internalIP:   internalIP.Unmap(),
		externalIP:   externalIP,
		clientPubKey: clientPubKey,
		allowedIPs:   normalized,
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
// Both internalIP and allowedIPs are pre-normalized at session creation.
func (s *Session) IsSourceAllowed(srcIP netip.Addr) bool {
	src := srcIP.Unmap()
	if src == s.internalIP {
		return true
	}
	for _, prefix := range s.allowedIPs {
		if prefix.Contains(src) {
			return true
		}
	}
	return false
}
