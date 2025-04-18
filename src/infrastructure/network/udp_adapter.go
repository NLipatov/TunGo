package network

import "net"

type UdpAdapter struct {
	Conn        net.UDPConn
	Addr        net.UDPAddr
	InitialData []byte
}

func (ua *UdpAdapter) Write(data []byte) (int, error) {
	return ua.Conn.WriteToUDP(data, &ua.Addr)
}

func (ua *UdpAdapter) Read(buffer []byte) (int, error) {
	if ua.InitialData != nil {
		n := copy(buffer, ua.InitialData)
		ua.InitialData = nil
		return n, nil
	}

	n, _, err := ua.Conn.ReadFromUDP(buffer)
	return n, err
}

func (ua *UdpAdapter) Close() error {
	return ua.Conn.Close()
}
