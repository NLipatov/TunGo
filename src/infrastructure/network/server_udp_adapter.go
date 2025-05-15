package network

import (
	"net"
	"net/netip"
	"tungo/application"
)

type ServerUdpAdapter struct {
	UdpConn  *net.UDPConn
	AddrPort netip.AddrPort

	//read buffers
	buf [65_547]byte
	oob [1024]byte
}

func NewUdpAdapter(UdpConn *net.UDPConn, AddrPort netip.AddrPort) application.ConnectionAdapter {
	return &ServerUdpAdapter{
		UdpConn:  UdpConn,
		AddrPort: AddrPort,
	}
}

func (ua *ServerUdpAdapter) Write(data []byte) (int, error) {
	return ua.UdpConn.WriteToUDPAddrPort(data, ua.AddrPort)
}

func (ua *ServerUdpAdapter) Read(buffer []byte) (int, error) {
	n, _, _, _, err := ua.UdpConn.ReadMsgUDPAddrPort(ua.buf[:], ua.oob[:])
	if err != nil {
		return 0, err
	}
	copy(buffer[:n], ua.buf[:n])
	return n, nil
}

func (ua *ServerUdpAdapter) Close() error {
	return ua.UdpConn.Close()
}
