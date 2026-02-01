package session

import (
	"net/netip"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// Session represents a single encrypted session between a VPN client and server.
type Session struct {
	crypto     connection.Crypto
	fsm        rekey.FSM
	internalIP netip.Addr
	externalIP netip.AddrPort
}

func NewSession(
	crypto connection.Crypto,
	fsm rekey.FSM,
	internalIP netip.Addr,
	externalIP netip.AddrPort,
) connection.Session {
	return &Session{
		crypto:     crypto,
		fsm:        fsm,
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

func (s Session) RekeyController() rekey.FSM {
	return s.fsm
}
