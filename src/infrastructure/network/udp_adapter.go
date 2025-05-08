package network

import (
	"net"
)

type UdpAdapter struct {
	Conn        *net.UDPConn
	Addr        *net.UDPAddr
	InitialData []byte

	//read buffers
	buf [65_547]byte
	oob [1024]byte
}

func (ua *UdpAdapter) Write(data []byte) (int, error) {
	return ua.Conn.WriteTo(data, ua.Addr)
}

func (ua *UdpAdapter) Read(buffer []byte) (int, error) {
	if ua.InitialData != nil {
		n := copy(buffer, ua.InitialData)
		ua.InitialData = nil
		return n, nil
	}

	n, _, _, _, err := ua.Conn.ReadMsgUDPAddrPort(ua.buf[:], ua.oob[:])
	copy(buffer[:n], ua.buf[:n])
	return n, err
}

func (ua *UdpAdapter) Close() error {
	return ua.Conn.Close()
}
