package network

import (
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/network/keepalive"
)

const MaxPacketSizeBytes = 65535

type Packet struct {
	Length      uint32 //number of bytes in packet
	Payload     []byte
	IsKeepAlive bool
}

// bytes to packet
func (p *Packet) Decode(data []byte, session *ChaCha20.Session) (*Packet, error) {
	length := uint32(len(data))

	// shortcut - keep-alive messages are not encrypted
	if length == 9 && keepalive.IsKeepAlive(data) {
		return &Packet{
			Length:      length,
			Payload:     data,
			IsKeepAlive: true,
		}, nil
	}

	decrypted, err := session.Decrypt(data)
	if err != nil {
		return nil, err
	}

	return &Packet{
		Length:  uint32(len(decrypted)),
		Payload: data,
	}, nil
}

// packet to bytes
func (p *Packet) Encode(payload []byte) (*Packet, error) {
	length := uint32(len(payload))
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, length)

	return &Packet{
		Length:      length,
		Payload:     append(lengthBuf, payload...),
		IsKeepAlive: false,
	}, nil
}

func (p *Packet) EncodeUDP(payload []byte) (*Packet, error) {
	length := uint32(len(payload))

	return &Packet{
		Length:      length,
		Payload:     payload,
		IsKeepAlive: false,
	}, nil
}
