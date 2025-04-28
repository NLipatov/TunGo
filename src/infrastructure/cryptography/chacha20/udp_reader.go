package chacha20

import (
	"encoding/binary"
	"io"
)

type UdpReader struct {
	reader io.Reader
}

func NewUdpReader(reader io.Reader) *UdpReader {
	return &UdpReader{
		reader: reader,
	}
}

func (r *UdpReader) Read(buffer []byte) (int, error) {
	// reserves first 12 bytes for encryption overhead (12 bytes nonce)
	n, err := r.reader.Read(buffer[12:])
	if err != nil {
		return 0, err
	}

	// writes total frame length (header + payload) into buffer[0:4]
	binary.BigEndian.PutUint32(buffer[:12], uint32(n+12))

	return n + 12, nil
}
