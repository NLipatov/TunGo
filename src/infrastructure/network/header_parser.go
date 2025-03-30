package network

import (
	"fmt"
	"log"
)

type HeaderParser interface {
	Parse(data []byte) (Header, error)
}

type BaseHeaderParser struct {
	preAllocatedIPHeaderV4 headerV4
	preAllocatedIPHeaderV6 headerV6
}

func NewBaseHeaderParser() HeaderParser {
	return &BaseHeaderParser{}
}

func (p *BaseHeaderParser) Parse(data []byte) (Header, error) {
	ipVersion := data[0] >> 4
	switch ipVersion {
	case 4:
		v4ParseErr := ParseIPv4Header(data, &p.preAllocatedIPHeaderV4)
		if v4ParseErr != nil {
			log.Printf("failed to parse IPv4 header: %v", v4ParseErr)
			return nil, v4ParseErr
		}
		return &p.preAllocatedIPHeaderV4, nil
	case 6:
		v6ParseErr := ParseIPv6Header(data, &p.preAllocatedIPHeaderV6)
		if v6ParseErr != nil {
			log.Printf("failed to parse IPv4 header: %v", v6ParseErr)
			return nil, v6ParseErr
		}
		return &p.preAllocatedIPHeaderV6, nil
	default:
		return nil, fmt.Errorf("unsupported IP version: %v", ipVersion)
	}
}
