package ip

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type HeaderBuilder struct{}

func NewHeaderBuilder() *HeaderBuilder {
	return &HeaderBuilder{}
}

// BuildIPv4Packet builds a fresh IPv4 packet = header(20B) + payload.
// It sets:
//   - Version/IHL=4/5, TOS=0, ID=0, DF/frag=0
//   - TTL=ttl, Protocol=proto, HeaderChecksum computed
//   - Src/Dst per args
func (h HeaderBuilder) BuildIPv4Packet(src, dst netip.Addr, proto, ttl uint8, payload []byte) ([]byte, error) {
	if !src.Is4() || !dst.Is4() {
		return nil, fmt.Errorf("IPv4 addresses required")
	}
	total := ipv4.HeaderLen + len(payload)
	out := make([]byte, total)

	out[0] = 0x45 // Version=4, IHL=5
	// TOS=0
	binary.BigEndian.PutUint16(out[2:4], uint16(total)) // Total Length
	// ID=0
	// Flags/FragOff=0 (no fragmentation)
	out[8] = ttl
	out[9] = proto
	// checksum will be computed below

	srcBytes := src.As4()
	dstBytes := dst.As4()
	copy(out[12:16], srcBytes[:])
	copy(out[16:20], dstBytes[:])

	copy(out[ipv4.HeaderLen:], payload)

	// Compute checksum over the header
	binary.BigEndian.PutUint16(out[10:12], 0)
	cs := h.ipv4HeaderChecksum(out[:ipv4.HeaderLen])
	binary.BigEndian.PutUint16(out[10:12], cs)

	return out, nil
}

func (h HeaderBuilder) ipv4HeaderChecksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i < len(hdr); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(hdr[i : i+2]))
	}
	// fold carries
	for (sum >> 16) != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return ^uint16(sum)
}

// BuildIPv6Packet builds a fresh IPv6 packet = base header (40B) + payload.
// It sets:
//   - Version=6 (TC/Flow=0), PayloadLen=len(payload)
//   - NextHeader=nh, HopLimit=hl
//   - Src/Dst per args
//
// No extension headers, no fragmentation.
func (HeaderBuilder) BuildIPv6Packet(src, dst netip.Addr, nh, hl uint8, payload []byte) ([]byte, error) {
	if !src.Is6() || !dst.Is6() {
		return nil, fmt.Errorf("IPv6 addresses required")
	}
	total := ipv6.HeaderLen + len(payload)
	out := make([]byte, total)

	out[0] = 0x60                                              // Version 6, TC/Flow=0
	binary.BigEndian.PutUint16(out[4:6], uint16(len(payload))) // Payload Length
	out[6] = nh
	out[7] = hl

	srcBytes := src.As16()
	dstBytes := dst.As16()
	copy(out[8:24], srcBytes[:])
	copy(out[24:40], dstBytes[:])
	copy(out[ipv6.HeaderLen:], payload)

	return out, nil
}
