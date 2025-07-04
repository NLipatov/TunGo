package tcp_chacha20

import (
	"net"
	"net/netip"
	"tungo/application"
)

type Session struct {
	conn net.Conn
	// CryptographyService handles packet encryption and decryption.
	CryptographyService application.CryptographyService
	// internalIP is the client's VPN-assigned IPv4 address (e.g. 10.0.1.3).
	internalIP netip.Addr
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45) and port (e.g. 1754).
	externalIP netip.AddrPort
}

func (s Session) InternalIP() netip.Addr {
	return s.internalIP
}

func (s Session) ExternalIP() netip.AddrPort {
	return s.externalIP
}
