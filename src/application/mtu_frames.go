package application

import (
	"encoding/binary"
	"net/netip"
)

// ServiceIP is the reserved TEST-NET address used for in-band MTU signaling.
var ServiceIP = netip.MustParseAddr("192.0.2.1")

const (
	// MTUProbeType marks a service frame as an MTU probe.
	MTUProbeType byte = 1
	// MTUAckType marks a service frame as an acknowledgement of a probe.
	MTUAckType byte = 2
)

// BuildMTUPacket writes a minimal IPv4 packet destined to ServiceIP with the
// given service type into the provided buffer. Size specifies the total IP
// packet length. If the size is below the minimum header+payload, it is rounded
// up. The returned slice references the same backing array truncated to the
// packet size.
func BuildMTUPacket(buf []byte, typ byte, size int) []byte {
	const headerLen = 20 // minimal IPv4 header length
	if size < headerLen+1 {
		size = headerLen + 1
	}
	if cap(buf) < size {
		buf = make([]byte, size)
	}
	b := buf[:size]
	// Version=4, IHL=5 (20 bytes)
	b[0] = 0x45
	// Total length
	binary.BigEndian.PutUint16(b[2:4], uint16(size))
	// TTL
	b[8] = 64
	// Destination address
	dest := ServiceIP.As4()
	copy(b[16:20], dest[:])
	// Service payload
	b[headerLen] = typ
	return b
}
