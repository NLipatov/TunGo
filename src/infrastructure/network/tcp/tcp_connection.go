package tcp

import (
	"net"
	"net/netip"
	"tungo/application"
	"tungo/infrastructure/network/tcp/adapters"
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

	// NewTcpAdapter is used to handle framing specific of tcp transport
	framingAdapter := adapters.NewTcpAdapter(conn)

	return framingAdapter, nil
}
