package network

import (
	"io"
	"net"
	"tungo/application"
)

// ClientUDPAdapter - single goroutine only client udp adapter
type ClientUDPAdapter struct {
	conn                        *net.UDPConn
	buf                         [MaxPacketLengthBytes]byte
	readDeadline, writeDeadline Deadline
}

func NewClientUDPAdapter(
	conn *net.UDPConn,
	readDeadline, writeDeadline Deadline) application.ConnectionAdapter {
	return &ClientUDPAdapter{
		conn:          conn,
		writeDeadline: writeDeadline,
		readDeadline:  readDeadline,
	}
}

func (c *ClientUDPAdapter) Write(buffer []byte) (int, error) {
	if err := c.conn.SetWriteDeadline(c.writeDeadline.Time()); err != nil {
		return 0, err
	}

	return c.conn.Write(buffer)
}

func (c *ClientUDPAdapter) Read(buffer []byte) (int, error) {
	if err := c.conn.SetReadDeadline(c.readDeadline.Time()); err != nil {
		return 0, err
	}

	n, _, _, _, err := c.conn.ReadMsgUDPAddrPort(c.buf[:], nil)
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
