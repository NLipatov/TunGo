package udp_chacha20

import (
	"net"
	"tungo/application"
)

// session represents a single encrypted session between a VPN client and server.
type session struct {
	// udpConn is the underlying UDP connection used for sending and receiving packets.
	udpConn *net.UDPConn
	// udpAddr is the remote client's UDP address (IP and port).
	udpAddr *net.UDPAddr
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
