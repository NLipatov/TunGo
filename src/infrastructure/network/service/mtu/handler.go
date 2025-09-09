package mtu

import (
	"encoding/binary"

	"tungo/application/network/ip"
	domain "tungo/domain/network/serviceframe"
	nip "tungo/infrastructure/network/ip"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const HLOrTTL = uint8(64)

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

func NewHandler(fp FrameParser, hb ip.HeaderBuilder, sf *domain.Frame) *DefaultHandler {
	return &DefaultHandler{
		frameParser:   fp,
		headerBuilder: hb,
		serviceFrame:  sf,
	}
}

func (d *DefaultHandler) Handle(data []byte) []byte {
	// TryParse must give us src, dst, ipVersion, protocol (nh for v6).
	info, ok := d.frameParser.TryParse(data)
	if !ok {
		return data
	}

	// Slice service-frame payload (IP payload).
	payload := d.sliceServicePayload(data, info)
	if payload == nil {
		return data
	}

	// Parse SF and ensure it is MTUProbe.
	if err := d.serviceFrame.UnmarshalBinary(payload); err != nil {
		return data
	}
	if d.serviceFrame.Kind() != domain.KindMTUProbe {
		return data
	}

	// Echo back the probe's body length (2 bytes, BE) as ACK payload.
	mtuBE := make([]byte, 2)
	binary.BigEndian.PutUint16(mtuBE, uint16(len(d.serviceFrame.Body())))

	ack, err := domain.NewFrame(domain.V1, domain.KindMTUAck, domain.FlagNone, mtuBE)
	if err != nil {
		return data
	}
	wire, err := ack.MarshalBinary()
	if err != nil {
		return data
	}

	// Reply: swap dst/src; keep protocol/nh from the incoming packet to satisfy builder contract.
	switch info.ipVersion {
	case nip.V4:
		reply, err := d.headerBuilder.BuildIPv4Packet(info.dst.Unmap(), info.src.Unmap(), info.protocol, HLOrTTL, wire)
		if err != nil {
			return data
		}
		return reply
	case nip.V6:
		reply, err := d.headerBuilder.BuildIPv6Packet(info.dst, info.src, info.protocol, HLOrTTL, wire)
		if err != nil {
			return data
		}
		return reply
	default:
		return data
	}
}

// sliceServicePayload returns the IP payload part where the SF sits.
func (d *DefaultHandler) sliceServicePayload(data []byte, info FrameInfo) []byte {
	switch info.ipVersion {
	case nip.V4:
		if len(data) < ipv4.HeaderLen {
			return nil
		}
		ihl := int(data[0]&0x0F) << 2
		if ihl < ipv4.HeaderLen || len(data) < ihl {
			return nil
		}
		return data[ihl:]
	case nip.V6:
		if len(data) < ipv6.HeaderLen {
			return nil
		}
		return data[ipv6.HeaderLen:]
	default:
		return nil
	}
}
