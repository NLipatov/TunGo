package pipes

import (
	"encoding/binary"
	"fmt"
	"io"
	"tungo/network"
)

type TCPReaderPipe struct {
	pipe Pipe
	from io.Reader
	to   io.Writer
}

func NewTCPReaderPipe(pipe Pipe, from io.Reader, to io.Writer) Pipe {
	return &TCPReaderPipe{
		pipe: pipe,
		from: from,
		to:   to,
	}
}

func (trp *TCPReaderPipe) Pass(data []byte) error {
	_, readLengthErr := io.ReadFull(trp.from, data[:4])
	if readLengthErr != nil {
		return fmt.Errorf("read length error: %v", readLengthErr)
	}

	length := binary.BigEndian.Uint32(data[:4])
	if length < 4 || length > network.IPPacketMaxSizeBytes {
		return fmt.Errorf("invalid packet Length: %d", length)
	}

	_, readPacketErr := io.ReadFull(trp.from, data[:length])
	if readPacketErr != nil {
		return fmt.Errorf("failed to read packet from connection: %v", readPacketErr)
	}

	return trp.pipe.Pass(data[:length])
}
