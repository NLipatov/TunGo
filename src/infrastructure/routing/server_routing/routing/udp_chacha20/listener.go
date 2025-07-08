package udp_chacha20

import (
	"fmt"
	"net"
	"net/netip"
	"tungo/application"
)

type Listener struct {
	socket application.Socket
	udp    *net.UDPConn
}

func NewListener(socket application.Socket) application.Listener {
	return &Listener{
		socket: socket,
	}
}

func (u *Listener) Listen() (application.UdpListenerConn, error) {
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

func (u *Listener) Read(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error) {
	return u.udp.ReadMsgUDPAddrPort(b, oob)
}
