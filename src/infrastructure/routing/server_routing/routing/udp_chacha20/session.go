package udp_chacha20

import (
	"net/netip"
	"tungo/application"
)

// Session represents a single encrypted Session between a VPN client and server.
type Session struct {
	connectionAdapter application.ConnectionAdapter
	remoteAddrPort    netip.AddrPort
	// CryptographyService handles packet encryption and decryption.
	CryptographyService application.CryptographyService
	// internalIP is the client's VPN-assigned IPv4 address (e.g. 10.0.1.3).
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45).
	internalIP, externalIP []byte
}

func (s Session) InternalIP() []byte {
	return s.internalIP
}

func (s Session) ExternalIP() []byte {
	return s.externalIP
}
