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

	// allowedAddrs are the additional single-host addresses (/32, /128)
	// for O(1) source IP validation. The internalIP is checked separately.
	allowedAddrs map[netip.Addr]struct{}

	// allowedSubnets holds non-host prefixes (e.g. /24) as fallback.
	// Empty in typical deployments; only populated if broader subnets are configured.
	allowedSubnets []netip.Prefix
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
// Single-host prefixes (/32, /128) are expanded into a map for O(1)
// source IP validation in IsSourceAllowed.
func NewSessionWithAuth(
	crypto connection.Crypto,
	fsm rekey.FSM,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
	clientPubKey []byte,
	allowedIPs []netip.Prefix,
) *Session {
	addrs := make(map[netip.Addr]struct{}, len(allowedIPs))
	var subnets []netip.Prefix
	for _, p := range allowedIPs {
		if p.IsSingleIP() {
			addrs[p.Addr().Unmap()] = struct{}{}
		} else {
			subnets = append(subnets, netip.PrefixFrom(p.Addr().Unmap(), p.Bits()))
		}
	}
	return &Session{
		crypto:         crypto,
		fsm:            fsm,
		internalIP:     internalIP.Unmap(),
		externalIP:     externalIP,
		clientPubKey:   clientPubKey,
		allowedAddrs:   addrs,
		allowedSubnets: subnets,
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

// AllowedAddrs returns the additional addresses this client may use as source IP.
func (s *Session) AllowedAddrs() map[netip.Addr]struct{} {
	return s.allowedAddrs
}

// IsSourceAllowed checks if the given source IP is allowed for this session.
// O(1) for single-host entries (equality + map lookup); falls back to prefix
// scan only for non-host subnets (typically none in production).
func (s *Session) IsSourceAllowed(srcIP netip.Addr) bool {
	src := srcIP.Unmap()
	if src == s.internalIP {
		return true
	}
	if _, ok := s.allowedAddrs[src]; ok {
		return true
	}
	for _, prefix := range s.allowedSubnets {
		if prefix.Contains(src) {
			return true
		}
	}
	return false
}
