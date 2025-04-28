package chacha20

import (
	"encoding/binary"
	"io"
)

type TcpReader struct {
	reader io.Reader
}

func NewTcpReader(reader io.Reader) *TcpReader {
	return &TcpReader{
		reader: reader,
	}
}

func (r *TcpReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer[4:])
	if err != nil {
		return 0, err
	}

	// put payload length into first 4 bytes
	binary.BigEndian.PutUint32(buffer[:4], uint32(n+4))

	return n + 4, nil
}
