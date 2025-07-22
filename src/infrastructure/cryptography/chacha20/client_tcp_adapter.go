package chacha20

import (
	"encoding/binary"
	"fmt"
	"io"
	"tungo/application"
)

type ClientTCPAdapter struct {
	conn application.ConnectionAdapter
}

func NewClientTCPAdapter(conn application.ConnectionAdapter) *ClientTCPAdapter {
	return &ClientTCPAdapter{conn: conn}
}

// Write writes payload with 4-byte length prefix
func (c *ClientTCPAdapter) Write(payload []byte) (int, error) {
	length := uint32(len(payload))
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)

	if _, err := c.conn.Write(header); err != nil {
		return 0, err
	}
	return c.conn.Write(payload)
}

// Read reads payload framed with 4-byte length prefix
func (c *ClientTCPAdapter) Read(buffer []byte) (int, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return 0, fmt.Errorf("failed to read length prefix: %v", err)
	}
	length := int(binary.BigEndian.Uint32(header))
	if length > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	if _, err := io.ReadFull(c.conn, buffer[:length]); err != nil {
		return 0, fmt.Errorf("failed to read payload: %v", err)
	}
	return length, nil
}

func (c *ClientTCPAdapter) Close() error {
	return c.conn.Close()
}
