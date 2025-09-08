package mtu

import (
	"net/netip"
	"tungo/application/network/ip"
	domain "tungo/domain/network/serviceframe"
	nip "tungo/infrastructure/network/ip"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type FrameInfo struct {
	src, dst  netip.Addr
	ipVersion nip.Version
	protocol  uint8
}

type FrameParser interface {
	TryParse(data []byte) (FrameInfo, bool)
}

type DefaultFrameParser struct {
	headerParser ip.HeaderParser
	serviceFrame *domain.Frame
}

func NewDefaultFrameParser() *DefaultFrameParser {
	return &DefaultFrameParser{
		headerParser: nip.NewHeaderParser(),
		serviceFrame: &domain.Frame{},
	}
}

func NewFrameParser(
	headerParser ip.HeaderParser,
	frame *domain.Frame,
) *DefaultFrameParser {
	return &DefaultFrameParser{
		headerParser: headerParser,
		serviceFrame: frame,
	}
}

func (f *DefaultFrameParser) TryParse(data []byte) (FrameInfo, bool) {
	rawIPVersion, rawIPVersionErr := f.headerParser.Version(data)
	if rawIPVersionErr != nil {
		// not an ip packet, passthrough
		return FrameInfo{}, false
	}
	ipVersion, ipVersionErr := nip.FromUint8(rawIPVersion)
	if ipVersionErr != nil {
		return FrameInfo{}, false
	}

	var payload []byte
	var protocol uint8
	switch ipVersion {
	case nip.V4:
		// Minimal safe parsing for v4
		if len(data) < ipv4.HeaderLen {
			return FrameInfo{}, false
		}
		ihl := int(data[0]&0x0F) * 4
		if ihl < ipv4.HeaderLen || len(data) < ihl {
			return FrameInfo{}, false
		}
		protocol = data[9]
		payload = data[ihl:]
	case nip.V6:
		if len(data) < ipv6.HeaderLen {
			return FrameInfo{}, false
		}
		protocol = data[6]
		payload = data[ipv6.HeaderLen:]
	default:
		return FrameInfo{}, false
	}

	// Quick SF signature check to avoid allocations.
	if len(payload) < domain.HeaderSize ||
		payload[0] != domain.MagicSF[0] || payload[1] != domain.MagicSF[1] {
		return FrameInfo{}, false
	}

	// Validate SF
	if err := f.serviceFrame.UnmarshalBinary(payload); err != nil {
		return FrameInfo{}, false
	}

	// We only handle MTUProbe here; others pass-through.
	if f.serviceFrame.Kind() != domain.KindMTUProbe {
		return FrameInfo{}, false
	}

	// Extract src/dst from the IP header
	dst, dstErr := f.headerParser.DestinationAddress(data)
	if dstErr != nil {
		return FrameInfo{}, false
	}
	src, srcErr := f.headerParser.SourceAddress(data)
	if srcErr != nil {
		return FrameInfo{}, false
	}

	return FrameInfo{
		src:       src,
		dst:       dst,
		ipVersion: ipVersion,
		protocol:  protocol,
	}, true
}
