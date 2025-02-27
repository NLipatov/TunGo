package pipes

import (
	"encoding/binary"
	"log"
	"tungo/network"
)

type TcpFrameBufferReadPipe struct {
	next Pipe
}

func NewTcpFrameBufferReadPipe(next Pipe) Pipe {
	return &TcpFrameBufferReadPipe{
		next: next,
	}
}

func (p *TcpFrameBufferReadPipe) Pass(data []byte) error {
	var length = binary.BigEndian.Uint32(data[:4])
	if length < 4 || length > network.IPPacketMaxSizeBytes {
		log.Printf("invalid packet Length: %d", length)
	}

	return p.next.Pass(data[:length])
}
