package network

import (
	"encoding/binary"
	"tungo/handshake/ChaCha20"
	"tungo/network/keepalive"
)

type UDPPacket struct {
	Nonce       *ChaCha20.Nonce
	Payload     *[]byte
	IsKeepAlive bool
}

type Packet struct {
	Length      uint32 //number of bytes in packet
	Payload     []byte
	IsKeepAlive bool
}

// DecodeTCP bytes to packet
func (p *Packet) DecodeTCP(data []byte) (*Packet, error) {
	length := uint32(len(data))

	// shortcut - keep-alive messages are not encrypted
	if length == 9 && keepalive.IsKeepAlive(data) {
		return &Packet{
			Length:      length,
			Payload:     data,
			IsKeepAlive: true,
		}, nil
	}

	return &Packet{
		Length:  length,
		Payload: data,
	}, nil
}

// EncodeTCP packet to bytes
func (p *Packet) EncodeTCP(payload []byte) (*Packet, error) {
	length := uint32(len(payload))
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, length)

	return &Packet{
		Length:      length,
		Payload:     append(lengthBuf, payload...),
		IsKeepAlive: false,
	}, nil
}

func (p *Packet) EncodeUDP(payload []byte, nonce *ChaCha20.Nonce) (*UDPPacket, error) {
	high := make([]byte, 4)
	binary.BigEndian.PutUint32(high, nonce.High)

	low := make([]byte, 8)
	binary.BigEndian.PutUint64(low, nonce.Low)

	data := make([]byte, 0, len(high)+len(low)+len(payload))
	data = append(data, high...)
	data = append(data, low...)
	data = append(data, payload...)

	return &UDPPacket{
		Payload:     &data,
		IsKeepAlive: false,
		Nonce:       nonce,
	}, nil
}

func (p *Packet) DecodeUDP(data []byte) (*UDPPacket, error) {
	length := uint32(len(data))

	// shortcut - keep-alive messages are not encrypted
	if length == 9 && keepalive.IsKeepAlive(data) {
		return &UDPPacket{
			Payload:     &data,
			IsKeepAlive: true,
		}, nil
	}

	high := binary.BigEndian.Uint32(data[:4])
	low := binary.BigEndian.Uint64(data[4:12])
	payload := data[12:]

	return &UDPPacket{
		Payload: &payload,
		Nonce: &ChaCha20.Nonce{
			High: high,
			Low:  low,
		},
	}, nil
}
