package udp_chacha20

import (
	"net/netip"
	"tungo/application"
)

// session represents a single encrypted session between a VPN client and server.
type session struct {
	connectionAdapter application.ConnectionAdapter
	remoteAddrPort    netip.AddrPort
	// CryptographyService handles packet encryption and decryption.
	CryptographyService application.CryptographyService
	// internalIP is the client's VPN-assigned IPv4 address (e.g. 10.0.1.3).
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45).
	internalIP, externalIP []byte
}

func (s session) InternalIP() []byte {
	return s.internalIP
}

func (s session) ExternalIP() []byte {
	return s.externalIP
}
