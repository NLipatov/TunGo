package network

import "net"

type (
	//ConnectionAdapter provides a single and trivial API for any supported transports
	ConnectionAdapter interface {
		Write([]byte) (int, error)
		Read([]byte) (int, error)
	}
	UdpAdapter struct {
		Conn        net.UDPConn
		Addr        net.UDPAddr
		InitialData []byte
	}
	TcpAdapter struct {
		Conn net.Conn
	}
)

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

func (ta *TcpAdapter) Write(data []byte) (int, error) {
	return ta.Conn.Write(data)
}

func (ta *TcpAdapter) Read(buffer []byte) (int, error) {
	n, err := ta.Conn.Read(buffer)
	return n, err
}
