package network

import (
	"fmt"
	"golang.org/x/net/ipv4"
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

	if len(header) < ipv4.HeaderLen {
		return fmt.Errorf("invalid packet size: too small (%d bytes)", len(header))
	}

	if ipVersion := header[0] >> 4; ipVersion != 4 {
		return fmt.Errorf("invalid packet version: got version%d, expected version 4(ipv4)", ipVersion)
	}

	// 16, 17, 18, 19 - destination address bytes
	copy(resultBuffer[:4], header[16:20])
	resultBuffer = resultBuffer[:4]

	return nil
}
