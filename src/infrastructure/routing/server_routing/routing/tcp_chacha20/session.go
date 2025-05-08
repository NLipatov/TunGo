package tcp_chacha20

import (
	"net"
	"tungo/application"
)

type session struct {
	conn net.Conn
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
