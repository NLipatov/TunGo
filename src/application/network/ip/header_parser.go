package ip

import "net/netip"

type HeaderParser interface {
	// Version extracts IP packet version (4 or 6).
	Version(header []byte) (uint8, error)

	// Protocol extracts protocol from IPv4/IPv6 header.
	Protocol(header []byte) (uint8, error)

	// DestinationAddress extracts destination netip.Addr from IPv4/IPv6 header.
	DestinationAddress(header []byte) (netip.Addr, error)

	// SourceAddress extracts source netip.Addr from IPv4/IPv6 header.
	SourceAddress(header []byte) (netip.Addr, error)
}
