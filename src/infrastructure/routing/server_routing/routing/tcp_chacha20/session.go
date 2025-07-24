package tcp_chacha20

import (
	"net/netip"
	"tungo/application"
)

type Session struct {
	connectionAdapter application.ConnectionAdapter
	// cryptographyService handles packet encryption and decryption.
	cryptographyService application.CryptographyService
	// internalIP is the client's VPN-assigned IPv4 address (e.g. 10.0.1.3).
	internalIP netip.Addr
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45) and port (e.g. 1754).
	externalIP netip.AddrPort
}

func NewSession(
	connectionAdapter application.ConnectionAdapter,
	cryptographyService application.CryptographyService,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
) application.Session {
	return &Session{
		connectionAdapter:   connectionAdapter,
		cryptographyService: cryptographyService,
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

func (s Session) CryptographyService() application.CryptographyService {
	return s.cryptographyService
}

func (s Session) ConnectionAdapter() application.ConnectionAdapter {
	return s.connectionAdapter
}
