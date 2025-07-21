package network

import (
	"encoding/binary"
	"tungo/application"
)

type ClientTCPAdapter struct {
	conn application.ConnectionAdapter
}

func NewClientTCPAdapter(conn application.ConnectionAdapter) *ClientTCPAdapter {
	return &ClientTCPAdapter{
		conn: conn,
	}
}

func (c *ClientTCPAdapter) Write(p []byte) (n int, err error) {
	length := uint32(len(p[4:]))
	binary.BigEndian.PutUint32(p[:4], length)

	return c.conn.Write(p)
}

func (c *ClientTCPAdapter) Read(buffer []byte) (int, error) {
	return c.conn.Read(buffer)
}

func (c *ClientTCPAdapter) Close() error {
	return c.conn.Close()
}
