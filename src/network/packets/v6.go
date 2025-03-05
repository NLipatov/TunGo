package packets

import (
	"encoding/binary"
	"fmt"
	"net"
)

// https://en.wikipedia.org/wiki/IPv6_packet#Fixed_header

type IPv6Header struct {
	Version       uint8
	TrafficClass  uint8
	FlowLabel     uint32
	PayloadLength uint16
	NextHeader    uint8
	HopLimit      uint8
	SourceIP      net.IP
	DestinationIP net.IP
}

func (h *IPv6Header) GetDestinationIP() net.IP {
	return h.DestinationIP
}

func (h *IPv6Header) GetSourceIP() net.IP {
	return h.SourceIP
}

func ParseIPv6Header(packet []byte, header *IPv6Header) error {
	if len(packet) < 40 {
		return fmt.Errorf("invalid packet length for IPv6")
	}

	versionTrafficClass := packet[0] // First 4 bits for version and next 4 for traffic class
	version := versionTrafficClass >> 4
	if version != 6 {
		return fmt.Errorf("not an IPv6 packet")
	}

	flowLabel := binary.BigEndian.Uint32([]byte{0, packet[1] & 0x0F, packet[2], packet[3]})
	header.Version = version
	header.TrafficClass = (versionTrafficClass & 0x0F << 4) | (packet[1] >> 4)
	header.FlowLabel = flowLabel
	header.PayloadLength = binary.BigEndian.Uint16(packet[4:6])
	header.NextHeader = packet[6]
	header.HopLimit = packet[7]
	header.SourceIP = packet[8:24]
	header.DestinationIP = packet[24:40]

	return nil
}
