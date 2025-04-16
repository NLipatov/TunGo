package network

import "net"

type TcpAdapter struct {
	Conn net.Conn
}

func (ta *TcpAdapter) Write(data []byte) (int, error) {
	return ta.Conn.Write(data)
}

func (ta *TcpAdapter) Read(buffer []byte) (int, error) {
	n, err := ta.Conn.Read(buffer)
	return n, err
}

func (ta *TcpAdapter) Close() error {
	return ta.Conn.Close()
}
