package udp_listener

import (
	"net"
	"net/netip"
)

type Listener interface {
	ListenUDP() (*net.UDPConn, error)
	ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error)
}
