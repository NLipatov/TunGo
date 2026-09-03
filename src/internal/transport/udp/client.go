package udp

import (
	"io"
	"net"
	"time"
	"tungo/internal/config/settings"
)

// clientConn - single goroutine only client UDP adapter
type clientConn struct {
	conn          *net.UDPConn
	buf           [settings.DefaultEthernetMTU + settings.UDPChacha20Overhead]byte
	writeDeadline time.Duration
}

func NewClientConn(
	conn *net.UDPConn,
	writeDeadline time.Duration) io.ReadWriteCloser {
	return &clientConn{
		conn:          conn,
		writeDeadline: writeDeadline,
	}
}

func (c *clientConn) Write(buffer []byte) (int, error) {
	deadline := time.Time{}
	if c.writeDeadline > 0 {
		deadline = time.Now().Add(c.writeDeadline)
	}
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return 0, err
	}

	return c.conn.Write(buffer)
}

func (c *clientConn) Read(buffer []byte) (int, error) {
	// Fast path: packet-loop buffers are already max-sized, so read directly
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

func (c *clientConn) Close() error {
	return c.conn.Close()
}
