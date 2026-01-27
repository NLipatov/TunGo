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
	epoch := Epoch(binary.BigEndian.Uint16(data[0:2]))
	counterHigh := binary.BigEndian.Uint16(data[2:4])
	counterLow := binary.BigEndian.Uint64(data[4:12])
	payload := data[12:]

	return &UDPPacket{
		Payload: payload,
		Nonce: &Nonce{
			epoch:       epoch,
			counterHigh: counterHigh,
			counterLow:  counterLow,
		},
	}, nil
}
