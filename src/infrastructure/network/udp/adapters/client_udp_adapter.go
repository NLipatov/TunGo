package adapters

import (
	"io"
	"net"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"
)

// ClientUDPAdapter - single goroutine only client UDP adapter
type ClientUDPAdapter struct {
	conn                        *net.UDPConn
	buf                         [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	readDeadline, writeDeadline time.Duration
}

func NewClientUDPAdapter(
	conn *net.UDPConn,
	readDeadline, writeDeadline time.Duration) connection.Transport {
	return &ClientUDPAdapter{
		conn:          conn,
		writeDeadline: writeDeadline,
		readDeadline:  readDeadline,
	}
}

func (c *ClientUDPAdapter) Write(buffer []byte) (int, error) {
	deadline := time.Time{}
	if c.writeDeadline > 0 {
		deadline = time.Now().Add(c.writeDeadline)
	}
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return 0, err
	}

	return c.conn.Write(buffer)
}

func (c *ClientUDPAdapter) Read(buffer []byte) (int, error) {
	deadline := time.Time{}
	if c.readDeadline > 0 {
		deadline = time.Now().Add(c.readDeadline)
	}
	if err := c.conn.SetReadDeadline(deadline); err != nil {
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
