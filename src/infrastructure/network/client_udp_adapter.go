package network

import (
	"net"
	"time"
	"tungo/application"
)

type ClientUdpAdapter struct {
	conn *net.UDPConn

	// read buffers
	buf [65_547]byte
	oob [1024]byte
}

func NewClientUdpAdapter(conn *net.UDPConn) application.ConnectionAdapter {
	return &ClientUdpAdapter{conn: conn}
}

func (c *ClientUdpAdapter) Write(bytes []byte) (int, error) {
	if err := c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return 0, err
	}
	return c.conn.Write(bytes) // без аллокаций
}

func (c *ClientUdpAdapter) Read(buffer []byte) (int, error) {
	n, _, _, _, err := c.conn.ReadMsgUDPAddrPort(c.buf[:], c.oob[:])
	if err != nil {
		return 0, err
	}
	copy(buffer, c.buf[:n])
	return n, nil
}

func (c *ClientUdpAdapter) Close() error {
	return c.conn.Close()
}
