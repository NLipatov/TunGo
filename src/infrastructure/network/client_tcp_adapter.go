package network

import (
	"encoding/binary"
	"fmt"
	"io"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
)

type ClientTCPAdapter struct {
	conn    application.ConnectionAdapter
	encoder chacha20.TCPEncoder
}

func NewClientTCPAdapter(
	conn application.ConnectionAdapter,
	encoder chacha20.TCPEncoder,
) *ClientTCPAdapter {
	return &ClientTCPAdapter{
		conn:    conn,
		encoder: encoder,
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
	// read length prefix (it's stored in first 4 bytes and contains information about payload length in bytes)
	_, lenPrefixErr := io.ReadFull(c.conn, buffer[:4])
	if lenPrefixErr != nil {
		return 0, fmt.Errorf("failed to read length prefix: %v", lenPrefixErr)
	}

	//read packet length from 4-byte length prefix
	var payloadSizeBytesUint32 = binary.BigEndian.Uint32(buffer[:4])
	if payloadSizeBytesUint32 > uint32(MaxPacketLengthBytes) {
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
