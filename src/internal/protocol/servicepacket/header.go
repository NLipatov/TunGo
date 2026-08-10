package servicepacket

import (
	"io"
)

// v1 service packet wire:
// byte 0: Prefix
// byte 1: Version (1)
// byte 2: Header type
// byte 3+: body
const (
	Prefix    byte = 0xFF
	VersionV1 byte = 1
)

type HeaderType uint8

const (
	Unknown HeaderType = iota
	_                  // was SessionReset; keep subsequent wire values stable
	RekeyInit
	RekeyAck
	Ping
	Pong
	EpochExhausted // server → client: cannot rekey, please reconnect
	RekeyInitV2
	RekeyAckV2
)

// Parse detects service_packet packets in-place without allocations.
// Returns (type, ok). Never returns an error on the fast path.
func Parse(pkt []byte) (HeaderType, bool) {
	if len(pkt) < 3 {
		return Unknown, false
	}
	if pkt[0] == Prefix && pkt[1] == VersionV1 {
		return HeaderType(pkt[2]), true
	}
	return Unknown, false
}

// Encode writes a v1 service packet header into the first three bytes of dst.
func Encode(t HeaderType, dst []byte) error {
	if len(dst) < 3 {
		return io.ErrShortBuffer
	}
	dst[0] = Prefix
	dst[1] = VersionV1
	dst[2] = byte(t)
	return nil
}
