package tcp_chacha20

import (
	"net/netip"
	"tungo/application/network/connection"
	"tungo/application/network/rekey"
)

type Session struct {
	connectionAdapter connection.Transport
	// cryptographyService handles packet encryption and decryption.
	cryptographyService connection.Crypto
	// internalIP is the client's VPN-assigned IPv4 address (e.g. 10.0.1.3).
	internalIP netip.Addr
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45) and port (e.g. 1754).
	externalIP      netip.AddrPort
	rekeyController *rekey.Controller
}

func NewSession(
	connectionAdapter connection.Transport,
	cryptographyService connection.Crypto,
	rekeyController *rekey.Controller,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
) connection.Session {
	return &Session{
		connectionAdapter:   connectionAdapter,
		cryptographyService: cryptographyService,
		rekeyController:     rekeyController,
		internalIP:          internalIP,
		externalIP:          externalIP,
	}
}

func (s Session) InternalAddr() netip.Addr {
	return s.internalIP
}

func (s Session) ExternalAddrPort() netip.AddrPort {
	return s.externalIP
}

func (s Session) Crypto() connection.Crypto {
	return s.cryptographyService
}

func (s Session) RekeyController() *rekey.Controller {
	return s.rekeyController
}

func (s Session) Transport() connection.Transport {
	return s.connectionAdapter
}
