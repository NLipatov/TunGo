package service_packet

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
	ErrInvalidHeader = errors.New("invalid header")
)

type HeaderType uint8

const (
	Unknown HeaderType = iota
	SessionReset
	RekeyInit
	RekeyAck
)

// TryParseHeader detects service_packet packets in-place without allocations.
// Returns (type, ok). Never returns an error on the fast path.
func TryParseHeader(pkt []byte) (HeaderType, bool) {
	switch len(pkt) {
	case 1: // legacy: <type>
		typ := HeaderType(pkt[0])
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
		typ := HeaderType(pkt[2])
		switch typ {
		case SessionReset:
			return typ, true
		default:
			return Unknown, false
		}
	case RekeyPacketLen: // v1 rekey packet with payload
		if pkt[0] != Prefix || pkt[1] != VersionV1 {
			return Unknown, false
		}
		switch HeaderType(pkt[2]) {
		case RekeyInit:
			return RekeyInit, true
		case RekeyAck:
			return RekeyAck, true
		default:
			return Unknown, false
		}
	default:
		return Unknown, false
	}
}

// EncodeLegacyHeader writes legacy single-byte encoding.
func EncodeLegacyHeader(headerType HeaderType, dst []byte) ([]byte, error) {
	if len(dst) < 1 {
		return nil, io.ErrShortBuffer
	}
	switch headerType {
	case SessionReset:
		dst[0] = byte(headerType)
		return dst[:1], nil
	default:
		return nil, ErrInvalidHeader
	}
}

// EncodeV1Header writes framed encoding: 0xFF <ver=1> <type>.
func EncodeV1Header(headerType HeaderType, dst []byte) ([]byte, error) {
	switch headerType {
	case SessionReset:
		if len(dst) < 3 {
			return nil, io.ErrShortBuffer
		}
		dst[0] = Prefix
		dst[1] = VersionV1
		dst[2] = byte(headerType)
		return dst[:3], nil
	case RekeyInit:
		if len(dst) < RekeyPacketLen {
			return nil, io.ErrShortBuffer
		}
		dst[0] = Prefix
		dst[1] = VersionV1
		dst[2] = byte(headerType)
		return dst[:RekeyPacketLen], nil
	case RekeyAck:
		if len(dst) < RekeyPacketLen {
			return nil, io.ErrShortBuffer
		}
		dst[0] = Prefix
		dst[1] = VersionV1
		dst[2] = byte(headerType)
		return dst[:RekeyPacketLen], nil
	default:
		return nil, ErrInvalidHeader
	}
}
