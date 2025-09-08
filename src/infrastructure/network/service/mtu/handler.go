package mtu

import (
	"fmt"
	"tungo/application/network/ip"
	domain "tungo/domain/network/serviceframe"
	nip "tungo/infrastructure/network/ip"
)

const (
	// HLOrTTL is a value that's called hop limit(hl) in ipv6 spec and time-to-live(ttl) in ipv4 spec
	HLOrTTL = uint8(64)
)

type DefaultHandler struct {
	frameParser   FrameParser
	headerBuilder ip.HeaderBuilder
	serviceFrame  *domain.Frame
}

func NewDefaultHandler() *DefaultHandler {
	return &DefaultHandler{
		frameParser:   NewDefaultFrameParser(),
		headerBuilder: nip.NewHeaderBuilder(),
		serviceFrame:  &domain.Frame{},
	}
}

func NewHandler(
	frameParser FrameParser,
	headerBuilder ip.HeaderBuilder,
	serviceFrame *domain.Frame,
) *DefaultHandler {
	return &DefaultHandler{
		frameParser:   frameParser,
		headerBuilder: headerBuilder,
		serviceFrame:  serviceFrame,
	}
}

func (d *DefaultHandler) Handle(data []byte) []byte {
	frameInfo, notAServiceFrame := d.frameParser.TryParse(data)
	if !notAServiceFrame {
		return data
	}

	// Swap dst with src. This client's packet will be going backward, i.e. from server to client.
	reply, err := d.generateReply(frameInfo)
	if err != nil {
		return data
	}
	return reply
}

func (d *DefaultHandler) generateReply(info FrameInfo) ([]byte, error) {
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
	switch info.ipVersion {
	case 4:
		return d.headerBuilder.BuildIPv4Packet(info.src.Unmap(), info.dst.Unmap(), info.protocol, HLOrTTL, wire)
	case 6:
		return d.headerBuilder.BuildIPv6Packet(info.src, info.dst, info.protocol, HLOrTTL, wire)
	default:
		return nil, fmt.Errorf("unsupported IP header version %d", info.ipVersion)
	}
}
