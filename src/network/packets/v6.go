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

func ParseIPv6Header(packet []byte) (*IPv6Header, error) {
	if len(packet) < 40 {
		return nil, fmt.Errorf("invalid packet length for IPv6")
	}

	versionTrafficClass := packet[0] // First 4 bits for version and next 4 for traffic class
	version := versionTrafficClass >> 4
	if version != 6 {
		return nil, fmt.Errorf("not an IPv6 packet")
	}

	trafficClass := (versionTrafficClass & 0x0F << 4) | (packet[1] >> 4)
	flowLabel := binary.BigEndian.Uint32([]byte{0, packet[1] & 0x0F, packet[2], packet[3]})

	return &IPv6Header{
		Version:       version,
		TrafficClass:  trafficClass,
		FlowLabel:     flowLabel,
		PayloadLength: binary.BigEndian.Uint16(packet[4:6]),
		NextHeader:    packet[6],
		HopLimit:      packet[7],
		SourceIP:      net.IP(packet[8:24]),
		DestinationIP: net.IP(packet[24:40]),
	}, nil
}
