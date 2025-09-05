package ip

import (
	"fmt"
	"net/netip"

	appip "tungo/application/network/ip"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Compile-time interface check.
var _ appip.HeaderParser = (*HeaderParser)(nil)

type HeaderParser struct{}

func NewHeaderParser() appip.HeaderParser { return &HeaderParser{} }

// DestinationAddress parses an IPv4/IPv6 header and returns the destination address.
// IPv4: header[16:20]. IPv6: header[24:40].
func (HeaderParser) DestinationAddress(header []byte) (netip.Addr, error) {
	if len(header) < 1 {
		return netip.Addr{}, fmt.Errorf("invalid packet: empty header")
	}
	version := header[0] >> 4 // high nibble

	switch version {
	case 4:
		if len(header) < ipv4.HeaderLen {
			return netip.Addr{}, fmt.Errorf("invalid IPv4 header: too small (%d bytes)", len(header))
		}
		ihl := int(header[0]&0x0F) * 4
		if ihl < ipv4.HeaderLen {
			return netip.Addr{}, fmt.Errorf("invalid IPv4 header: IHL=%d (<%d)", ihl, ipv4.HeaderLen)
		}
		if len(header) < ihl {
			return netip.Addr{}, fmt.Errorf("invalid IPv4 header: truncated (len=%d < IHL=%d)", len(header), ihl)
		}
		return netip.AddrFrom4([4]byte{header[16], header[17], header[18], header[19]}), nil

	case 6:
		if len(header) < ipv6.HeaderLen {
			return netip.Addr{}, fmt.Errorf("invalid IPv6 header: too small (%d bytes)", len(header))
		}
		var a16 [16]byte // stays on stack; no heap alloc
		copy(a16[:], header[24:40])
		return netip.AddrFrom16(a16), nil

	default:
		return netip.Addr{}, fmt.Errorf("invalid IP version: %d", version)
	}
}

// SourceAddress parses an IPv4/IPv6 header and returns the source address.
// IPv4: header[12:16]. IPv6: header[8:24].
func (HeaderParser) SourceAddress(header []byte) (netip.Addr, error) {
	if len(header) < 1 {
		return netip.Addr{}, fmt.Errorf("invalid packet: empty header")
	}
	version := header[0] >> 4 // high nibble

	switch version {
	case 4:
		if len(header) < ipv4.HeaderLen {
			return netip.Addr{}, fmt.Errorf("invalid IPv4 header: too small (%d bytes)", len(header))
		}
		ihl := int(header[0]&0x0F) * 4
		if ihl < ipv4.HeaderLen {
			return netip.Addr{}, fmt.Errorf("invalid IPv4 header: IHL=%d (<%d)", ihl, ipv4.HeaderLen)
		}
		if len(header) < ihl {
			return netip.Addr{}, fmt.Errorf("invalid IPv4 header: truncated (len=%d < IHL=%d)", len(header), ihl)
		}
		return netip.AddrFrom4([4]byte{header[12], header[13], header[14], header[15]}), nil

	case 6:
		if len(header) < ipv6.HeaderLen {
			return netip.Addr{}, fmt.Errorf("invalid IPv6 header: too small (%d bytes)", len(header))
		}
		var a16 [16]byte // stays on stack; no heap alloc
		copy(a16[:], header[8:24])
		return netip.AddrFrom16(a16), nil

	default:
		return netip.Addr{}, fmt.Errorf("invalid IP version: %d", version)
	}
}

func (HeaderParser) Protocol(header []byte) (uint8, error) {
	if len(header) < 1 {
		return 0, fmt.Errorf("invalid packet: empty header")
	}
	version := header[0] >> 4
	switch version {
	case 4:
		if len(header) < ipv4.HeaderLen {
			return 0, fmt.Errorf("invalid IPv4 header: too small (%d bytes)", len(header))
		}
		ihl := int(header[0]&0x0F) * 4
		if ihl < ipv4.HeaderLen {
			return 0, fmt.Errorf("invalid IPv4 header: IHL=%d (<%d)", ihl, ipv4.HeaderLen)
		}
		if len(header) < ihl {
			return 0, fmt.Errorf("invalid IPv4 header: truncated (len=%d < IHL=%d)", len(header), ihl)
		}
		return header[9], nil
	case 6:
		if len(header) < ipv6.HeaderLen {
			return 0, fmt.Errorf("invalid IPv6 header: too small (%d bytes)", len(header))
		}
		return header[6], nil
	default:
		return 0, fmt.Errorf("invalid IP version: %d", version)
	}
}

func (HeaderParser) Version(header []byte) (uint8, error) {
	if len(header) < 1 {
		return 0, fmt.Errorf("invalid packet: empty header")
	}
	version := header[0] >> 4
	if version == 4 || version == 6 {
		return version, nil
	}
	return 0, fmt.Errorf("invalid IP version: %d", version)
}
