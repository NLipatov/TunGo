package ip

import (
	"encoding/binary"
	"fmt"
	"net"
)

// https://en.wikipedia.org/wiki/IPv4#Packet_structure

type headerV4 struct {
	version        uint8
	Ihl            uint8 // Internet Header Length in 32-bit words (1 word = 4 bytes)
	dscp           uint8 // Differentiated Services Code Point (QoS field)
	totalLength    uint16
	identification uint16 // Unique identifier for the packet group (for fragmentation)
	flags          uint8  // Control flags for fragmentation (3 bits)
	fragmentOffset uint16
	ttl            uint8
	protocol       uint8
	headerChecksum uint16
	sourceIP       net.IP
	destinationIP  net.IP
}

func (h *headerV4) GetDestinationIP() net.IP {
	return h.destinationIP
}

func (h *headerV4) GetSourceIP() net.IP {
	return h.sourceIP
}

func ParseIPv4Header(packet []byte, header *headerV4) error {
	if len(packet) < 20 {
		return fmt.Errorf("invalid packet length")
	}

	versionIHL := packet[0]    // The first byte contains both the version (first 4 bits) and the IHL (next 4 bits)
	version := versionIHL >> 4 // Extracting the first 4 bits (version)
	ihl := versionIHL & 0x0F   // Extracting the next 4 bits (IHL)

	if version != 4 {
		return fmt.Errorf("not a IPv4 packet")
	}

	if len(packet) < int(ihl)*4 {
		return fmt.Errorf("invalid IPv4 header length")
	}

	header.version = version
	header.Ihl = ihl
	header.dscp = packet[1]
	header.totalLength = binary.BigEndian.Uint16(packet[2:4])
	header.identification = binary.BigEndian.Uint16(packet[4:6])
	header.flags = packet[6] >> 5 // Extract the first 3 bits of the 6th byte (flags)
	header.fragmentOffset = binary.BigEndian.Uint16(packet[6:8]) & 0x1FFF
	header.ttl = packet[8]
	header.protocol = packet[9]
	header.headerChecksum = binary.BigEndian.Uint16(packet[10:12])
	header.sourceIP = net.IPv4(packet[12], packet[13], packet[14], packet[15])
	header.destinationIP = net.IPv4(packet[16], packet[17], packet[18], packet[19])

	return nil
}
