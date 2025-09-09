package adapters

import (
	"io"
	"net"
	"time"

	"tungo/application"
	"tungo/infrastructure/network"
	"tungo/infrastructure/settings"
)

// ClientUDPAdapter - single goroutine only client UDP adapter
type ClientUDPAdapter struct {
	conn                        *net.UDPConn
	buf                         [settings.MTU + settings.UDPChacha20Overhead]byte
	readDeadline, writeDeadline network.Timeout
}

func NewClientUDPAdapter(
	conn *net.UDPConn,
	readDeadline, writeDeadline network.Timeout) application.ConnectionAdapter {
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

func (c *ClientUDPAdapter) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *ClientUDPAdapter) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		c.readDeadline = 0
		return nil
	}
	d := time.Until(t)
	if d < 0 {
		d = 0
	}
	dl, err := network.NewDeadline(d)
	if err != nil {
		return err
	}
	c.readDeadline = dl
	return nil
}

func (c *ClientUDPAdapter) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		c.writeDeadline = 0
		return nil
	}
	d := time.Until(t)
	if d < 0 {
		d = 0
	}
	dl, err := network.NewDeadline(d)
	if err != nil {
		return err
	}
	c.writeDeadline = dl
	return nil
}
