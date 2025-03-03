package chacha20

import "encoding/binary"

type (
	TCPEncoder interface {
		Decode(data []byte) (*TCPPacket, error)
		Encode(payload []byte) (*TCPPacket, error)
	}
	DefaultTCPEncoder struct {
	}

	TCPPacket struct {
		Length  uint32 //number of bytes in packet
		Payload []byte
	}
)

func NewDefaultTCPEncoder() DefaultTCPEncoder {
	return DefaultTCPEncoder{}
}

func (p *DefaultTCPEncoder) Decode(data []byte) (*TCPPacket, error) {
	length := uint32(len(data))

	return &TCPPacket{
		Length:  length,
		Payload: data,
	}, nil
}

func (p *DefaultTCPEncoder) Encode(payload []byte) (*TCPPacket, error) {
	length := uint32(len(payload[4:]))
	binary.BigEndian.PutUint32(payload[:4], length)

	return &TCPPacket{
		Length:  length,
		Payload: payload,
	}, nil
}
