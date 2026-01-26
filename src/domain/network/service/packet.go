package service

import (
	"errors"
	"io"
)

const (
	Prefix    byte = 0xFF
	VersionV1 byte = 1
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
	case 3: // v1: <0xFF><ver><type>
		if pkt[0] != Prefix {
			return Unknown, false
		}
		ver := pkt[1]
		if ver == VersionV1 {
			typ := PacketType(pkt[2])
			switch typ {
			case SessionReset:
				return typ, true
			case RekeyInit:
				return RekeyInit, true
			default:
				return Unknown, false
			}
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
	if len(buffer) < 3 {
		return nil, io.ErrShortBuffer
	}
	switch typ {
	case SessionReset:
		buffer[0] = Prefix
		buffer[1] = VersionV1
		buffer[2] = byte(typ)
		return buffer[:3], nil
	default:
		return nil, ErrInvalidPacketType
	}
}
