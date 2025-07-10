package network

import (
	"io"
	"net/netip"
	"tungo/application"
)

type ServerUdpAdapter struct {
	Conn     application.UdpListenerConn
	AddrPort netip.AddrPort

	//read buffers
	buf [65_547]byte
	oob [1024]byte
}

func NewUdpAdapter(UdpConn application.UdpListenerConn, AddrPort netip.AddrPort) application.ConnectionAdapter {
	return &ServerUdpAdapter{
		Conn:     UdpConn,
		AddrPort: AddrPort,
	}
}

func (ua *ServerUdpAdapter) Write(data []byte) (int, error) {
	return ua.Conn.WriteToUDPAddrPort(data, ua.AddrPort)
}

func (ua *ServerUdpAdapter) Read(buffer []byte) (int, error) {
	n, _, _, _, err := ua.Conn.ReadMsgUDPAddrPort(ua.buf[:], ua.oob[:])
	if err != nil {
		return 0, err
	}

	if len(buffer) < n {
		return 0, io.ErrShortBuffer
	}

	copy(buffer[:n], ua.buf[:n])
	return n, nil
}

func (ua *ServerUdpAdapter) Close() error {
	return ua.Conn.Close()
}
