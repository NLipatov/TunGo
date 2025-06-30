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
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45).
	internalIP, externalIP netip.Addr
}

func (s Session) InternalIP() netip.Addr {
	return s.internalIP
}

func (s Session) ExternalIP() netip.Addr {
	return s.externalIP
}
