package network

import (
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
)

type ClientTCPAdapter struct {
	conn    application.ConnectionAdapter
	encoder chacha20.TCPEncoder
}

func NewClientTCPAdapter(conn application.ConnectionAdapter) *ClientTCPAdapter {
	return &ClientTCPAdapter{
		conn: conn,
	}
}

func (c *ClientTCPAdapter) Write(buffer []byte) (n int, err error) {
	encodingErr := c.encoder.Encode(buffer)
	if encodingErr != nil {
		return 0, encodingErr
	}

	return c.conn.Write(buffer)
}

func (c *ClientTCPAdapter) Read(buffer []byte) (int, error) {
	return c.conn.Read(buffer)
}

func (c *ClientTCPAdapter) Close() error {
	return c.conn.Close()
}
