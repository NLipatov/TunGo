package application

import (
	"net/netip"
)

type UdpListenerConn interface {
	Close() error
	ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error)
	SetReadBuffer(size int) error
	SetWriteBuffer(size int) error
	WriteToUDPAddrPort(data []byte, addr netip.AddrPort) (int, error)
}

type Listener interface {
	Listen() (UdpListenerConn, error)
	Read(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error)
}
