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
	n, err := r.reader.Read(buffer[12:])
	if err != nil {
		return 0, err
	}

	// put payload length into first 12 bytes
	binary.BigEndian.PutUint32(buffer[:12], uint32(n+12))

	return n, nil
}
