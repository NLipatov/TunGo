package network

import (
	"fmt"
)

type IPV4HeaderParser struct {
}

func NewIPV4HeaderParser() IPHeader {
	return &IPV4HeaderParser{}
}

func (h *IPV4HeaderParser) ParseDestinationAddressBytes(header, resultBuffer []byte) error {
	if len(resultBuffer) < 4 {
		return fmt.Errorf("invalid buffer size, expected 4 bytes, got %d", len(resultBuffer))
	}

	if len(header) < 20 {
		return fmt.Errorf("invalid packet size: %d", len(header))
	}

	ipVersion := header[0] >> 4
	if ipVersion != 4 {
		return fmt.Errorf("invalid packet version: %d", header[0])
	}

	// 16, 17, 18, 19 - destination address bytes
	copy(resultBuffer[:4], header[16:20])
	resultBuffer = resultBuffer[:4]

	return nil
}
