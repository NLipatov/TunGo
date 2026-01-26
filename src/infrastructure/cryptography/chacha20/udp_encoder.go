package chacha20

import (
	"encoding/binary"
)

type (
	UDPEncoder interface {
		Decode(data []byte) (*UDPPacket, error)
	}
	DefaultUDPEncoder struct {
	}
	UDPPacket struct {
		Nonce   *Nonce
		Payload []byte
	}
)

func (p *DefaultUDPEncoder) Decode(data []byte) (*UDPPacket, error) {
	low := binary.BigEndian.Uint64(data[:8])
	high := binary.BigEndian.Uint32(data[8:12])
	payload := data[12:]

	return &UDPPacket{
		Payload: payload,
		Nonce: &Nonce{
			high: high,
			low:  low,
		},
	}, nil
}
