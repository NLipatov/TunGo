package network

import (
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/network/keepalive"
	"fmt"
	"io"
	"net"
)

const MaxPacketSizeBytes = 65535

type Packet struct {
	Length      uint32 //number of bytes in packet
	Payload     []byte
	IsKeepAlive bool
}

func (p *Packet) Decode(conn net.Conn, buffer []byte, session *ChaCha20.Session) (*Packet, error) {
	//read packet length from 4-byte length prefix
	var length = binary.BigEndian.Uint32(buffer[:4])
	if length < 4 || length > MaxPacketSizeBytes {
		return nil, fmt.Errorf("invalid packet Length: %d", length)
	}

	//read n-bytes from connection
	_, err := io.ReadFull(conn, buffer[:length])
	if err != nil {
		return nil, err
	}

	// shortcut - keep-alive messages are not encrypted
	if length == 9 && keepalive.IsKeepAlive(buffer[:length]) {
		return &Packet{
			Length:      length,
			Payload:     buffer[:length],
			IsKeepAlive: true,
		}, nil
	}

	decrypted, err := session.Decrypt(buffer[:length])
	if err != nil {
		return nil, err
	}

	return &Packet{
		Length:  length,
		Payload: decrypted,
	}, nil
}

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
