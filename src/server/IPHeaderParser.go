package server

import (
	"fmt"
	"log"
	"tungo/network/packets"
)

type ipHeaderParser interface {
	Parse(data []byte) (packets.IPHeader, error)
}

type baseIpHeaderParser struct {
	preAllocatedIPHeaderV4 packets.IPHeaderV4
	preAllocatedIPHeaderV6 packets.IPHeaderV6
}

func newBaseIpHeaderParser() ipHeaderParser {
	return &baseIpHeaderParser{}
}

func (p *baseIpHeaderParser) Parse(data []byte) (packets.IPHeader, error) {
	ipVersion := data[0] >> 4
	switch ipVersion {
	case 4:
		v4ParseErr := packets.ParseIPv4Header(data, &p.preAllocatedIPHeaderV4)
		if v4ParseErr != nil {
			log.Printf("failed to parse IPv4 header: %v", v4ParseErr)
			return nil, v4ParseErr
		}
		return &p.preAllocatedIPHeaderV4, nil
	case 6:
		v6ParseErr := packets.ParseIPv6Header(data, &p.preAllocatedIPHeaderV6)
		if v6ParseErr != nil {
			log.Printf("failed to parse IPv4 header: %v", v6ParseErr)
			return nil, v6ParseErr
		}
		return &p.preAllocatedIPHeaderV6, nil
	default:
		return nil, fmt.Errorf("unsupported IP version: %v", ipVersion)
	}
}
