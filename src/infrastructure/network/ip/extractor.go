package ip

import (
	"net/netip"
)

const (
	// IPv4Version is the IP version number for IPv4.
	IPv4Version = 4

	// IPv6Version is the IP version number for IPv6.
	IPv6Version = 6

	// IPv4HeaderMinLen is the minimum IPv4 header length.
	IPv4HeaderMinLen = 20

	// IPv6HeaderLen is the fixed IPv6 header length.
	IPv6HeaderLen = 40

	// IPv4SrcOffset is the offset of source IP in IPv4 header.
	IPv4SrcOffset = 12

	// IPv4DstOffset is the offset of destination IP in IPv4 header.
	IPv4DstOffset = 16

	// IPv6SrcOffset is the offset of source IP in IPv6 header.
	IPv6SrcOffset = 8

	// IPv6DstOffset is the offset of destination IP in IPv6 header.
	IPv6DstOffset = 24
)

// ExtractSourceIP extracts the source IP address from an IP packet.
// Returns the IP address and true if successful, or an invalid address and false if the packet is malformed.
func ExtractSourceIP(packet []byte) (netip.Addr, bool) {
	return extractIPByOffsets(packet, IPv4SrcOffset, IPv6SrcOffset)
}

// ExtractDestIP extracts the destination IP address from an IP packet.
// Returns the IP address and true if successful, or an invalid address and false if the packet is malformed.
func ExtractDestIP(packet []byte) (netip.Addr, bool) {
	return extractIPByOffsets(packet, IPv4DstOffset, IPv6DstOffset)
}

// extractIPByOffsets extracts an IPv4 or IPv6 address from the packet using
// the provided source/destination offsets for each protocol version.
func extractIPByOffsets(packet []byte, ipv4Offset, ipv6Offset int) (netip.Addr, bool) {
	if len(packet) < 1 {
		return netip.Addr{}, false
	}

	version := packet[0] >> 4

	switch version {
	case IPv4Version:
		if len(packet) < IPv4HeaderMinLen {
			return netip.Addr{}, false
		}
		addr, ok := netip.AddrFromSlice(packet[ipv4Offset : ipv4Offset+4])
		return addr, ok

	case IPv6Version:
		if len(packet) < IPv6HeaderLen {
			return netip.Addr{}, false
		}
		addr, ok := netip.AddrFromSlice(packet[ipv6Offset : ipv6Offset+16])
		return addr, ok

	default:
		return netip.Addr{}, false
	}
}

// ExtractIPVersion extracts the IP version from a packet.
// Returns 4 for IPv4, 6 for IPv6, or 0 if the packet is empty.
func ExtractIPVersion(packet []byte) uint8 {
	if len(packet) < 1 {
		return 0
	}
	return packet[0] >> 4
}
