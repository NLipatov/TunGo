package framing

import "encoding/binary"

type TCPEncoder interface {
	Decode(data []byte, packet *TCPPacket) error
	Encode(buffer []byte) error
}

type DefaultTCPEncoder struct{}

type TCPPacket struct {
	Length  uint32
	Payload []byte
}

func NewDefaultTCPEncoder() TCPEncoder {
	return &DefaultTCPEncoder{}
}

func (e *DefaultTCPEncoder) Decode(data []byte, packet *TCPPacket) error {
	packet.Length = uint32(len(data))
	packet.Payload = data
	return nil
}

func (e *DefaultTCPEncoder) Encode(buffer []byte) error {
	length := uint32(len(buffer[4:]))
	binary.BigEndian.PutUint32(buffer[:4], length)
	return nil
}
