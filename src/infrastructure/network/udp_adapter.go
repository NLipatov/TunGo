package network

import (
	"net"
	"net/netip"
)

type UdpAdapter struct {
	UdpConn     *net.UDPConn
	AddrPort    netip.AddrPort
	InitialData []byte

	//read buffers
	buf [65_547]byte
	oob [1024]byte
}

func (ua *UdpAdapter) Write(data []byte) (int, error) {
	return ua.UdpConn.WriteToUDPAddrPort(data, ua.AddrPort)
}

func (ua *UdpAdapter) Read(buffer []byte) (int, error) {
	if ua.InitialData != nil {
		n := copy(buffer, ua.InitialData)
		ua.InitialData = nil
		return n, nil
	}

	n, _, _, _, err := ua.UdpConn.ReadMsgUDPAddrPort(ua.buf[:], ua.oob[:])
	if err != nil {
		return 0, err
	}
	copy(buffer[:n], ua.buf[:n])
	return n, nil
}

func (ua *UdpAdapter) Close() error {
	return ua.UdpConn.Close()
}
