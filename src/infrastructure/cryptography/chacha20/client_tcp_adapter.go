package chacha20

import (
	"encoding/binary"
	"fmt"
	"io"
	"tungo/application"
	"tungo/infrastructure/network"
)

type ClientTCPAdapter struct {
	conn    application.ConnectionAdapter
	encoder TCPEncoder
}

func NewClientTCPAdapter(
	conn application.ConnectionAdapter,
	encoder TCPEncoder,
) *ClientTCPAdapter {
	return &ClientTCPAdapter{
		conn:    conn,
		encoder: encoder,
	}
}

func (c *ClientTCPAdapter) Write(buffer []byte) (n int, err error) {
	length := uint32(len(buffer[4:]))
	binary.BigEndian.PutUint32(buffer[:4], length)

	return c.conn.Write(buffer)
}

func (c *ClientTCPAdapter) Read(buffer []byte) (int, error) {
	// read length prefix (it's stored in first 4 bytes and contains information about payload length in bytes)
	_, lenPrefixErr := io.ReadFull(c.conn, buffer[:4])
	if lenPrefixErr != nil {
		return 0, fmt.Errorf("failed to read length prefix: %v", lenPrefixErr)
	}

	//read packet length from 4-byte length prefix
	var payloadSizeBytesUint32 = binary.BigEndian.Uint32(buffer[:4])
	if payloadSizeBytesUint32 > uint32(network.MaxPacketLengthBytes) {
		return 0, fmt.Errorf("length prefix is invalid")
	}

	payloadLength := int(payloadSizeBytesUint32)
	if payloadLength > len(buffer) {
		return 0, io.ErrShortBuffer
	}

	//read n-bytes from connection
	_, lenPrefixErr = io.ReadFull(c.conn, buffer[:payloadLength])
	if lenPrefixErr != nil {
		return 0, fmt.Errorf("failed to read packet from connection: %s", lenPrefixErr)
	}

	return payloadLength, nil
}

func (c *ClientTCPAdapter) Close() error {
	return c.conn.Close()
}
