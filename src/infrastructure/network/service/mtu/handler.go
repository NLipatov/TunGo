package mtu

import (
	"fmt"
	"net/netip"
	"tungo/application/network/ip"
	domain "tungo/domain/network/serviceframe"
	nip "tungo/infrastructure/network/ip"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	// HLOrTTL is a value that's called hop limit(hl) in ipv6 spec and time-to-live(ttl) in ipv4 spec
	HLOrTTL = uint8(64)
)

type DefaultHandler struct {
	headerParser  ip.HeaderParser
	headerBuilder ip.HeaderBuilder
	serviceFrame  *domain.Frame
}

func NewDefaultHandler() *DefaultHandler {
	return &DefaultHandler{
		headerParser:  nip.NewHeaderParser(),
		headerBuilder: nip.NewHeaderBuilder(),
		serviceFrame:  &domain.Frame{},
	}
}

func NewHandler(
	headerParser ip.HeaderParser,
	headerBuilder ip.HeaderBuilder,
	serviceFrame *domain.Frame,
) *DefaultHandler {
	return &DefaultHandler{
		headerParser:  headerParser,
		headerBuilder: headerBuilder,
		serviceFrame:  serviceFrame,
	}
}

func (d *DefaultHandler) Handle(data []byte) []byte {
	version, versionErr := d.headerParser.Version(data)
	if versionErr != nil {
		// not an ip packet, passthrough
		return data
	}

	var payload []byte
	var proto uint8
	switch version {
	case 4:
		// Minimal safe parsing for v4
		if len(data) < ipv4.HeaderLen {
			return data
		}
		ihl := int(data[0]&0x0F) * 4
		if ihl < ipv4.HeaderLen || len(data) < ihl {
			return data
		}
		proto = data[9]
		payload = data[ihl:]
	case 6:
		if len(data) < ipv6.HeaderLen {
			return data
		}
		proto = data[6]
		payload = data[ipv6.HeaderLen:]
	default:
		return data
	}

	// Quick SF signature check to avoid allocations.
	if len(payload) < domain.HeaderSize ||
		payload[0] != domain.MagicSF[0] || payload[1] != domain.MagicSF[1] {
		return data
	}

	// Validate SF
	if err := d.serviceFrame.UnmarshalBinary(payload); err != nil {
		return data
	}

	// We only handle MTUProbe here; others pass-through.
	if d.serviceFrame.Kind() != domain.KindMTUProbe {
		return data
	}

	// Extract src/dst from the IP header
	dst, err := d.headerParser.DestinationAddress(data)
	if err != nil {
		return data
	}
	src, err := d.headerParser.SourceAddress(data)
	if err != nil {
		return data
	}

	// Swap dst with src. This client's packet will be going backward, i.e. from server to client.
	reply, err := d.generateReply(dst, src, version, proto, payload)
	if err != nil {
		return data
	}
	return reply
}

func (d *DefaultHandler) generateReply(src, dst netip.Addr, version, protocol uint8, _ []byte) ([]byte, error) {
	// generate ack service frame
	ack, ackErr := domain.NewFrame(
		domain.V1,
		domain.KindMTUAck,
		domain.FlagNone,
		nil,
	)
	if ackErr != nil {
		return nil, fmt.Errorf("failed to create MTU ack frame: %w", ackErr)
	}
	// generate wire
	wire, wireErr := ack.MarshalBinary()
	if wireErr != nil {
		return nil, fmt.Errorf("failed to marshal MTU ack frame: %w", wireErr)
	}
	switch version {
	case 4:
		return d.headerBuilder.BuildIPv4Packet(src.Unmap(), dst.Unmap(), protocol, HLOrTTL, wire)
	case 6:
		return d.headerBuilder.BuildIPv6Packet(src, dst, protocol, HLOrTTL, wire)
	default:
		return nil, fmt.Errorf("unsupported IP header version %d", version)
	}
}
