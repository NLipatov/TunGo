package chacha20

import (
	"encoding/binary"
	"io"
)

type UdpReader struct {
	buffer []byte
	reader io.Reader
}

func NewUdpReader(buffer []byte, reader io.Reader) *UdpReader {
	return &UdpReader{
		buffer: buffer,
		reader: reader,
	}
}

func (r *UdpReader) Read() error {
	n, err := r.reader.Read(r.buffer[12:])
	if err != nil {
		return err
	}

	// put payload length into first 12 bytes
	binary.BigEndian.PutUint32(r.buffer[:12], uint32(n+12))

	return nil
}
