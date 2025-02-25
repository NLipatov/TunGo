package chacha20

import (
	"encoding/binary"
)

type (
	UDPEncoder interface {
		Encode(payload []byte, nonce *Nonce) (*UDPPacket, error)
		Decode(data []byte) (*UDPPacket, error)
	}
	DefaultUDPEncoder struct {
	}
	UDPPacket struct {
		Nonce   *Nonce
		Payload []byte
	}
)

func (p *DefaultUDPEncoder) Encode(payload []byte, nonce *Nonce) (*UDPPacket, error) {
	data := make([]byte, len(payload)+12)
	copy(data[:12], nonce.Encode())
	copy(data[12:], payload)

	return &UDPPacket{
		Payload: data,
		Nonce:   nonce,
	}, nil
}

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
