package network

import (
	"encoding/binary"
	v4 "etha-tunnel/network/IPParsing"
	"fmt"
	"log"
	"net"
)

func Serve(ifName string) error {
	tunFile, err := OpenTunByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to open TUN interface: %v", err)
	}

	defer tunFile.Close()

	for {
		packet, readFromTunErr := ReadFromTun(tunFile)
		if readFromTunErr != nil {
			log.Printf("failed to read from TUN interface: %v", readFromTunErr)
			continue
		}

		ipHeader, ipV4ParsingErr := v4.ParseIPv4Header(packet)
		if ipV4ParsingErr != nil {
			log.Printf("failed to parse IPv4 header: %v", ipV4ParsingErr)
			continue
		}

		var conn net.Conn
		var dstPort uint16
		payloadStart := int(ipHeader.IHL) * 4

		switch ipHeader.Protocol {
		case 6: // TCP
			dstPort = binary.BigEndian.Uint16(packet[payloadStart : payloadStart+2])
			conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", ipHeader.DestinationIP.String(), dstPort))
		case 17: // UDP
			dstPort = binary.BigEndian.Uint16(packet[payloadStart : payloadStart+2])
			conn, err = net.Dial("udp", fmt.Sprintf("%s:%d", ipHeader.DestinationIP.String(), dstPort))
		default:
			return fmt.Errorf("unsupported protocol: %d", ipHeader.Protocol)
		}

		if err != nil {
			log.Printf("error dialing destination: %v", err)
			tryCloseConnection(conn)
			continue
		}

		_, err = conn.Write(packet[payloadStart:])
		if err != nil {
			log.Printf("error forwarding packet: %v", err)
			tryCloseConnection(conn)
			continue
		}

		if ipHeader.Protocol == 6 {
			buf := make([]byte, 1500)
			n, readingFromTunErr := conn.Read(buf)
			if readingFromTunErr != nil {
				log.Printf("error reading from external server: %v", readingFromTunErr)
				tryCloseConnection(conn)
				continue
			}

			_, writingToTunErr := tunFile.Write(buf[:n])
			if writingToTunErr != nil {
				log.Printf("error writing response to TUN: %v", writingToTunErr)
				tryCloseConnection(conn)
				continue
			}
		}
		tryCloseConnection(conn)
	}
}

func tryCloseConnection(conn net.Conn) {
	if conn != nil {
		conn.Close()
	}
}
