package ip

import "net/netip"

type HeaderParser interface {
	// DestinationAddress extracts destination netip.Addr from IPv4/IPv6.
	DestinationAddress(header []byte) (netip.Addr, error)
}
