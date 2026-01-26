package service

import (
	"errors"
	"io"
)

const (
	Prefix            byte = 0xFF
	VersionV1         byte = 1
	RekeyPublicKeyLen      = 32
	RekeyPacketLen         = 3 + RekeyPublicKeyLen
)

var (
	ErrInvalidPacketType = errors.New("invalid packet type")
)

type DefaultPacketHandler struct {
}

func NewDefaultPacketHandler() *DefaultPacketHandler {
	return &DefaultPacketHandler{}
}

// TryParseType detects service packets in-place without allocations.
// Returns (type, ok). Never returns an error on the fast path.
func (p *DefaultPacketHandler) TryParseType(pkt []byte) (PacketType, bool) {
	switch len(pkt) {
	case 1: // legacy: <type>
		typ := PacketType(pkt[0])
		switch typ {
		case SessionReset:
			return SessionReset, true
		default:
			return Unknown, false
		}
	case 3: // v1 header only: <0xFF><ver><type>
		if pkt[0] != Prefix || pkt[1] != VersionV1 {
			return Unknown, false
		}
		typ := PacketType(pkt[2])
		switch typ {
		case SessionReset, RekeyInit, RekeyAck:
			return typ, true
		default:
			return Unknown, false
		}
	case RekeyPacketLen: // v1 rekey packet with payload
		if pkt[0] != Prefix || pkt[1] != VersionV1 {
			return Unknown, false
		}
		switch PacketType(pkt[2]) {
		case RekeyInit:
			return RekeyInit, true
		case RekeyAck:
			return RekeyAck, true
		}
		return Unknown, false
	default:
		return Unknown, false
	}
}

// EncodeLegacy writes legacy single-byte encoding.
func (p *DefaultPacketHandler) EncodeLegacy(typ PacketType, buffer []byte) ([]byte, error) {
	if len(buffer) < 1 {
		return nil, io.ErrShortBuffer
	}
	switch typ {
	case SessionReset:
		buffer[0] = byte(typ)
		return buffer[:1], nil
	default:
		return nil, ErrInvalidPacketType
	}
}

// EncodeV1 writes framed encoding: 0xFF <ver=1> <type>.
func (p *DefaultPacketHandler) EncodeV1(typ PacketType, buffer []byte) ([]byte, error) {
	switch typ {
	case SessionReset:
		if len(buffer) < 3 {
			return nil, io.ErrShortBuffer
		}
		buffer[0] = Prefix
		buffer[1] = VersionV1
		buffer[2] = byte(typ)
		return buffer[:3], nil
	case RekeyInit:
		if len(buffer) < RekeyPacketLen {
			return nil, io.ErrShortBuffer
		}
		buffer[0] = Prefix
		buffer[1] = VersionV1
		buffer[2] = byte(typ)
		return buffer[:RekeyPacketLen], nil
	case RekeyAck:
		if len(buffer) < RekeyPacketLen {
			return nil, io.ErrShortBuffer
		}
		buffer[0] = Prefix
		buffer[1] = VersionV1
		buffer[2] = byte(typ)
		return buffer[:RekeyPacketLen], nil
	default:
		return nil, ErrInvalidPacketType
	}
}
