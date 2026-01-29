package tcp_chacha20

import (
	"net/netip"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type Session struct {
	connectionAdapter connection.Transport
	// cryptographyService handles packet encryption and decryption.
	cryptographyService connection.Crypto
	outbound            connection.Outbound
	// internalIP is the client's VPN-assigned IPv4 address (e.g. 10.0.1.3).
	internalIP netip.Addr
	// externalIP is the client's real-world IPv4 address (e.g. 51.195.101.45) and port (e.g. 1754).
	externalIP netip.AddrPort
	fsm        rekey.FSM
}

func NewSession(
	connectionAdapter connection.Transport,
	cryptographyService connection.Crypto,
	fsm rekey.FSM,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
) connection.Session {
	return &Session{
		connectionAdapter:   connectionAdapter,
		cryptographyService: cryptographyService,
		fsm:                 fsm,
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

func (s Session) RekeyController() rekey.FSM {
	return s.fsm
}

func (s Session) Transport() connection.Transport {
	return s.connectionAdapter
}
