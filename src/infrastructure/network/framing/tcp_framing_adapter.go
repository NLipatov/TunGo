package framing

import (
	"encoding/binary"
	"fmt"
	"io"
	"tungo/application"
)

// TCPFramingAdapter handles TCP framing using a 4-byte length prefix.
// All framing is internal; the caller deals only with pure payload bytes.
type TCPFramingAdapter struct {
	conn          application.ConnectionAdapter
	framingBuffer [4]byte // static buffer for framing, no allocations
}

// NewTCPFramingAdapter constructs a new TCP adapter with internal framing.
func NewTCPFramingAdapter(conn application.ConnectionAdapter) *TCPFramingAdapter {
	return &TCPFramingAdapter{conn: conn}
}

// Write sends a payload, automatically prepending a 4-byte length prefix.
// The input slice must contain only payload data, not a prefix.
func (c *TCPFramingAdapter) Write(payload []byte) (int, error) {
	binary.BigEndian.PutUint32(c.framingBuffer[:], uint32(len(payload)))

	// Write the length prefix first.
	if _, err := c.conn.Write(c.framingBuffer[:]); err != nil {
		return 0, err
	}
	// Write the actual payload bytes.
	return c.conn.Write(payload)
}

// Read reads a single framed packet into buffer.
// Returns the number of payload bytes read (without the prefix).
// If the buffer is too small, returns io.ErrShortBuffer.
func (c *TCPFramingAdapter) Read(buffer []byte) (int, error) {
	// Read the 4-byte length prefix.
	if _, err := io.ReadFull(c.conn, c.framingBuffer[:]); err != nil {
		return 0, fmt.Errorf("failed to read length prefix: %w", err)
	}
	payloadLen := int(binary.BigEndian.Uint32(c.framingBuffer[:]))
	if payloadLen > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	// Read the payload bytes.
	if _, err := io.ReadFull(c.conn, buffer[:payloadLen]); err != nil {
		return 0, fmt.Errorf("failed to read payload: %w", err)
	}
	return payloadLen, nil
}

// Close closes the underlying connection.
func (c *TCPFramingAdapter) Close() error {
	return c.conn.Close()
}
