package network

import (
	"net"
	"net/netip"
	"tungo/application"
)

type TCPDialer interface {
	Dial(network, address string) (net.Conn, error)
}

type TCPConnection struct {
	addrPort netip.AddrPort
	dialer   TCPDialer
}

func NewTCPConnection(
	addrPort netip.AddrPort,
) application.Connection {
	return &TCPConnection{
		addrPort: addrPort,
		dialer:   &net.Dialer{},
	}
}

func NewTCPConnectionWithDialer(
	addrPort netip.AddrPort,
	dialer TCPDialer,
) application.Connection {
	return &TCPConnection{
		addrPort: addrPort,
		dialer:   dialer,
	}
}

func (u *TCPConnection) Establish() (application.ConnectionAdapter, error) {
	conn, connErr := u.dialer.Dial("tcp", u.addrPort.String())
	if connErr != nil {
		return nil, connErr
	}

	return conn, nil
}
