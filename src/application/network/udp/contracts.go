package udp

import "net/netip"

type Reader interface {
	Close() error
	ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error)
	SetReadBuffer(size int) error
}

type Writer interface {
	Close() error
	SetWriteBuffer(size int) error
	WriteToUDPAddrPort(data []byte, addr netip.AddrPort) (int, error)
}

type Ingress interface {
	Close() error
	SetReadBuffer(size int) error
	ReadBatch(packets []Packet) (int, error)
}

type Listener interface {
	Reader
	Writer
}
