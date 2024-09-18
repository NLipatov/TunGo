package packets

import (
	"encoding/binary"
	"fmt"
	"net"
)

// https://en.wikipedia.org/wiki/IPv4#Packet_structure

type IPv4Header struct {
	Version        uint8
	IHL            uint8 // Internet IPHeader Length in 32-bit words (1 word = 4 bytes)
	DSCP           uint8 // Differentiated Services Code Point (QoS field)
	TotalLength    uint16
	Identification uint16 // Unique identifier for the packet group (for fragmentation)
	Flags          uint8  // Control flags for fragmentation (3 bits)
	FragmentOffset uint16
	TTL            uint8
	Protocol       uint8
	HeaderChecksum uint16
	SourceIP       net.IP
	DestinationIP  net.IP
}

func (h *IPv4Header) GetDestinationIP() net.IP {
	return h.DestinationIP
}

func ParseIPv4Header(packet []byte) (*IPv4Header, error) {
	if len(packet) < 20 {
		return nil, fmt.Errorf("invalid packet length")
	}

	versionIHL := packet[0]    // The first byte contains both the version (first 4 bits) and the IHL (next 4 bits)
	version := versionIHL >> 4 // Extracting the first 4 bits (version)
	ihl := versionIHL & 0x0F   // Extracting the next 4 bits (IHL)

	if version != 4 {
		return nil, fmt.Errorf("not a IPv4 packet")
	}

	if len(packet) < int(ihl)*4 {
		return nil, fmt.Errorf("invalid IPv4 header length")
	}

	return &IPv4Header{
		Version:        version,
		IHL:            ihl,
		DSCP:           packet[1],
		TotalLength:    binary.BigEndian.Uint16(packet[2:4]),
		Identification: binary.BigEndian.Uint16(packet[4:6]),
		Flags:          packet[6] >> 5, // Extract the first 3 bits of the 6th byte (flags)
		FragmentOffset: binary.BigEndian.Uint16(packet[6:8]) & 0x1FFF,
		TTL:            packet[8],
		Protocol:       packet[9],
		HeaderChecksum: binary.BigEndian.Uint16(packet[10:12]),
		SourceIP:       net.IPv4(packet[12], packet[13], packet[14], packet[15]),
		DestinationIP:  net.IPv4(packet[16], packet[17], packet[18], packet[19]),
	}, nil
}
