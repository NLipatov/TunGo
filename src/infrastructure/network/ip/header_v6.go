package ip

import (
	"encoding/binary"
	"fmt"
	"net"
)

// https://en.wikipedia.org/wiki/IPv6_packet#Fixed_header

type headerV6 struct {
	version       uint8
	trafficClass  uint8
	flowLabel     uint32
	payloadLength uint16
	nextHeader    uint8
	hopLimit      uint8
	sourceIP      net.IP
	destinationIP net.IP
}

func (h *headerV6) GetDestinationIP() net.IP {
	return h.destinationIP
}

func (h *headerV6) GetSourceIP() net.IP {
	return h.sourceIP
}

func ParseIPv6Header(packet []byte, header *headerV6) error {
	if len(packet) < 40 {
		return fmt.Errorf("invalid packet length for IPv6")
	}

	versionTrafficClass := packet[0] // First 4 bits for version and next 4 for traffic class
	version := versionTrafficClass >> 4
	if version != 6 {
		return fmt.Errorf("not an IPv6 packet")
	}

	flowLabel := binary.BigEndian.Uint32([]byte{0, packet[1] & 0x0F, packet[2], packet[3]})
	header.version = version
	header.trafficClass = (versionTrafficClass & 0x0F << 4) | (packet[1] >> 4)
	header.flowLabel = flowLabel
	header.payloadLength = binary.BigEndian.Uint16(packet[4:6])
	header.nextHeader = packet[6]
	header.hopLimit = packet[7]
	header.sourceIP = packet[8:24]
	header.destinationIP = packet[24:40]

	return nil
}
