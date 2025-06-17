package udp_listener

import (
	"fmt"
	"net"
	"net/netip"
	"tungo/application"
)

type UdpListener struct {
	socket application.Socket
	udp    *net.UDPConn
}

func NewUdpListener(socket application.Socket) Listener {
	return &UdpListener{
		socket: socket,
	}
}

func (u *UdpListener) ListenUDP() (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp", u.socket.StringAddr())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve udp addr: %s", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port: %s", err)
	}

	u.udp = conn

	return conn, nil
}

func (u *UdpListener) ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error) {
	return u.udp.ReadMsgUDPAddrPort(b, oob)
}
