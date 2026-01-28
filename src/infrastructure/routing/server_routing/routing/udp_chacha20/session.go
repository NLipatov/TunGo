package udp_chacha20

import (
	"net/netip"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// Session represents a single encrypted Session between a VPN client and server.
type Session struct {
	transport connection.Transport
	// crypto handles packet encryption and decryption.
	crypto connection.Crypto
	rekey  *rekey.Controller
	// internalIP is the client's VPN-assigned IPv4 address (e.g. 10.0.1.3).
	internalIP netip.Addr
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45) and port (e.g. 1754).
	externalIP netip.AddrPort
}

func NewSession(
	transport connection.Transport,
	crypto connection.Crypto,
	rekey *rekey.Controller,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
) connection.Session {
	return &Session{
		transport:  transport,
		crypto:     crypto,
		rekey:      rekey,
		internalIP: internalIP,
		externalIP: externalIP,
	}
}

func (s Session) InternalAddr() netip.Addr {
	return s.internalIP
}

func (s Session) ExternalAddrPort() netip.AddrPort {
	return s.externalIP
}

func (s Session) Crypto() connection.Crypto {
	return s.crypto
}

func (s Session) RekeyController() *rekey.Controller {
	return s.rekey
}

func (s Session) Transport() connection.Transport {
	return s.transport
}
