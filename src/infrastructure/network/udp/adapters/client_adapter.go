package adapters

import (
	"io"
	"net"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"
)

// ClientAdapter is a single-goroutine client transport adapter.
type ClientAdapter struct {
	conn                        *net.UDPConn
	buf                         [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	readDeadline, writeDeadline time.Duration
}

func NewClientAdapter(
	conn *net.UDPConn,
	readDeadline, writeDeadline time.Duration) connection.Transport {
	return &ClientAdapter{
		conn:          conn,
		writeDeadline: writeDeadline,
		readDeadline:  readDeadline,
	}
}

func (c *ClientAdapter) Write(buffer []byte) (int, error) {
	deadline := time.Time{}
	if c.writeDeadline > 0 {
		deadline = time.Now().Add(c.writeDeadline)
	}
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return 0, err
	}

	return c.conn.Write(buffer)
}

func (c *ClientAdapter) Read(buffer []byte) (int, error) {
	deadline := time.Time{}
	if c.readDeadline > 0 {
		deadline = time.Now().Add(c.readDeadline)
	}
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return 0, err
	}

	// Fast path: hot dataplane buffers are already max-sized, so read directly
	// into caller memory and avoid an extra copy.
	if len(buffer) >= len(c.buf) {
		n, _, _, _, err := c.conn.ReadMsgUDPAddrPort(buffer[:len(c.buf)], nil)
		if err != nil {
			return 0, err
		}
		return n, nil
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

func (c *ClientAdapter) Close() error {
	return c.conn.Close()
}
