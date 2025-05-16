package network

import (
	"io"
	"net"
	"time"
	"tungo/application"
)

// ClientUDPAdapter - single goroutine only client udp adapter
type ClientUDPAdapter struct {
	conn          *net.UDPConn
	buf           [MaxPacketLengthBytes]byte
	oob           [1024]byte
	readDeadline  time.Duration
	writeDeadline time.Duration
}

func NewClientUDPAdapter(conn *net.UDPConn, readDeadline, writeDeadline time.Duration) application.ConnectionAdapter {
	return &ClientUDPAdapter{
		conn:          conn,
		readDeadline:  readDeadline,
		writeDeadline: writeDeadline,
	}
}

func (c *ClientUDPAdapter) Write(buffer []byte) (int, error) {
	deadline := time.Now().Add(c.writeDeadline)
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return 0, err
	}

	return c.conn.Write(buffer)
}

func (c *ClientUDPAdapter) Read(buffer []byte) (int, error) {
	deadline := time.Now().Add(c.readDeadline)
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return 0, err
	}

	n, _, _, _, err := c.conn.ReadMsgUDPAddrPort(c.buf[:], c.oob[:])
	if err != nil {
		return 0, err
	}

	if len(buffer) < n {
		return 0, io.ErrShortBuffer
	}

	copy(buffer, c.buf[:n])
	return n, nil
}

func (c *ClientUDPAdapter) Close() error {
	return c.conn.Close()
}
