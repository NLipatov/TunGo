package chacha20

import (
	"encoding/binary"
)

type (
	UDPEncoder struct {
	}
	UDPPacket struct {
		Nonce   *Nonce
		Payload *[]byte
	}
)

func (p *UDPEncoder) Encode(payload []byte, nonce *Nonce) (*UDPPacket, error) {
	high := make([]byte, 4)
	binary.BigEndian.PutUint32(high, nonce.high)

	low := make([]byte, 8)
	binary.BigEndian.PutUint64(low, nonce.low)

	data := make([]byte, 0, len(high)+len(low)+len(payload))
	data = append(data, low...)
	data = append(data, high...)
	data = append(data, payload...)

	return &UDPPacket{
		Payload: &data,
		Nonce:   nonce,
	}, nil
}

func (p *UDPEncoder) Decode(data []byte) (*UDPPacket, error) {
	high := binary.BigEndian.Uint32(data[:4])
	low := binary.BigEndian.Uint64(data[4:12])
	payload := data[12:]

	return &UDPPacket{
		Payload: &payload,
		Nonce: &Nonce{
			high: high,
			low:  low,
		},
	}, nil
}
