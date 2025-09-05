package service

import (
	"fmt"
	"net/netip"
	"tungo/application/network/ip"
	domain "tungo/domain/network/serviceframe"
)

type Handler interface {
	Respond(src, dst netip.Addr, version, protocol uint8, serviceFrame []byte) ([]byte, error)
}

const (
	// HLOrTTL is a value that's called hop limit(hl) in ipv6 spec and time-to-live(ttl) in ipv4 spec
	HLOrTTL = uint8(64)
)

type MTUProbeHandler struct {
	headerBuilder ip.HeaderBuilder
	serverAddr    netip.Addr
}

func (m *MTUProbeHandler) Respond(src, dst netip.Addr, version, protocol uint8, _ []byte) ([]byte, error) {
	// generate ack service frame
	ack, ackErr := NewFrame(
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
		return m.headerBuilder.BuildIPv4Packet(src.Unmap(), dst.Unmap(), protocol, HLOrTTL, wire)
	case 6:
		return m.headerBuilder.BuildIPv6Packet(src, dst, protocol, HLOrTTL, wire)
	default:
		return nil, fmt.Errorf("unsupported IP header version %d", version)
	}
}
