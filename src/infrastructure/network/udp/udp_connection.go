package udp

import (
	"net"
	"net/netip"
	"tungo/application"
)

type UDPDialer interface {
	Dial(addr *net.UDPAddr) (*net.UDPConn, error)
}

type DefaultUDPDialer struct {
}

func (d *DefaultUDPDialer) Dial(addr *net.UDPAddr) (*net.UDPConn, error) {
	return net.DialUDP("udp", nil, addr)
}

type UDPConnection struct {
	addrPort netip.AddrPort
	dialer   UDPDialer
}

func NewUDPConnection(addrPort netip.AddrPort) application.Connection {
	return &UDPConnection{
		addrPort: addrPort,
		dialer:   &DefaultUDPDialer{},
	}
}

func (u *UDPConnection) Establish() (application.ConnectionAdapter, error) {
	conn, err := u.dialer.Dial(net.UDPAddrFromAddrPort(u.addrPort))
	if err != nil {
		return nil, err
	}

	return conn, nil
}
