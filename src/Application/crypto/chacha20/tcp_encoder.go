package chacha20

import "encoding/binary"

type (
	TCPEncoder struct {
	}

	TCPPacket struct {
		Length  uint32 //number of bytes in packet
		Payload []byte
	}
)

func (p *TCPEncoder) Decode(data []byte) (*TCPPacket, error) {
	length := uint32(len(data))

	return &TCPPacket{
		Length:  length,
		Payload: data,
	}, nil
}

func (p *TCPEncoder) Encode(payload []byte) (*TCPPacket, error) {
	length := uint32(len(payload))
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, length)

	return &TCPPacket{
		Length:  length,
		Payload: append(lengthBuf, payload...),
	}, nil
}
