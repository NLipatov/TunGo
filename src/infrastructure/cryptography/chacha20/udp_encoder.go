package chacha20

import (
	"encoding/binary"
	"fmt"
)

type (
	UDPEncoder interface {
		Decode(data []byte) (*UDPPacket, error)
	}
	DefaultUDPEncoder struct {
	}
	UDPPacket struct {
		KeyID   byte
		Nonce   *Nonce
		Payload []byte
	}
)

func (p *DefaultUDPEncoder) Decode(data []byte) (*UDPPacket, error) {
	if len(data) < 13 {
		return nil, fmt.Errorf("data too short")
	}
	keyID := data[0]
	low := binary.BigEndian.Uint64(data[1:9])
	high := binary.BigEndian.Uint32(data[9:13])
	payload := data[13:]

	return &UDPPacket{
		KeyID:   keyID,
		Payload: payload,
		Nonce: &Nonce{
			high: high,
			low:  low,
		},
	}, nil
}
