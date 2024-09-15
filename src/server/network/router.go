package network

import (
	"encoding/binary"
	"etha-tunnel/network/IPParsing"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"strings"
)

func Serve(ifName string) error {
	externalIfName, err := getDefaultInterface()
	if err != nil {
		return err
	}

	err = enableNAT(externalIfName)
	if err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}

	err = setupForwarding(ifName, externalIfName)

	tunFile, err := OpenTunByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to open TUN interface: %v", err)
	}

	defer tunFile.Close()
	defer disableNAT(externalIfName)
	defer clearForwarding(ifName, externalIfName)
	defer DeleteInterface(ifName)

	listener, err := net.Listen("tcp", ":8080") // Replace with desired port
	if err != nil {
		return fmt.Errorf("failed to listen on port: %v", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1500)
	for {
		// Read length
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			log.Printf("Failed to read from client: %v", err)
			return
		}
		length := binary.BigEndian.Uint32(buf[:4])
		if length > 1500 {
			log.Printf("Packet too large: %d", length)
			return
		}
		// Read packet
		_, err = io.ReadFull(conn, buf[:length])
		if err != nil {
			log.Printf("Failed to read from client: %v", err)
			return
		}
		packet := buf[:length]

		ipHeader, ipV4ParsingErr := IPParsing.ParseIPv4Header(packet)
		if ipV4ParsingErr != nil {
			log.Printf("Failed to parse IPv4 header: %v", ipV4ParsingErr)
			continue
		}

		var dstConn net.Conn
		var dstPort uint16
		payloadStart := int(ipHeader.IHL) * 4

		switch ipHeader.Protocol {
		case 6: // TCP
			dstPort = binary.BigEndian.Uint16(packet[payloadStart+2 : payloadStart+4]) // Destination port
			dstConn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", ipHeader.DestinationIP.String(), dstPort))
		case 17: // UDP
			dstPort = binary.BigEndian.Uint16(packet[payloadStart+2 : payloadStart+4]) // Destination port
			dstConn, err = net.Dial("udp", fmt.Sprintf("%s:%d", ipHeader.DestinationIP.String(), dstPort))
		default:
			log.Printf("Unsupported protocol: %d", ipHeader.Protocol)
			continue
		}

		if err != nil {
			log.Printf("Error dialing destination: %v", err)
			continue
		}

		_, err = dstConn.Write(packet[payloadStart:])
		if err != nil {
			log.Printf("Error forwarding packet: %v", err)
			dstConn.Close()
			continue
		}

		go func(dstConn net.Conn) {
			defer dstConn.Close()
			respBuf := make([]byte, 1500)
			for {
				n, err := dstConn.Read(respBuf[4:])
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading from destination: %v", err)
					}
					return
				}
				binary.BigEndian.PutUint32(respBuf[:4], uint32(n))
				_, err = conn.Write(respBuf[:4+n])
				if err != nil {
					log.Printf("Error sending response to client: %v", err)
					return
				}
			}
		}(dstConn)
	}
}

func enableNAT(iface string) error {
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", iface, "-j", "MASQUERADE")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable NAT on %s: %v, output: %s", iface, err, output)
	}
	return nil
}

func disableNAT(iface string) error {
	cmd := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-o", iface, "-j", "MASQUERADE")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable NAT on %s: %v, output: %s", iface, err, output)
	}
	return nil
}

func setupForwarding(tunIface, extIface string) error {
	cmd := exec.Command("iptables", "-A", "FORWARD", "-i", extIface, "-o", tunIface, "-m", "state", "--state",
		"RELATED,ESTABLISHED", "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule for extIface -> tunIface: %v, output: %s", err, output)
	}

	cmd = exec.Command("iptables", "-A", "FORWARD", "-i", tunIface, "-o", extIface, "-j", "ACCEPT")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule for tunIface -> extIface: %v, output: %s", err, output)
	}
	return nil
}

func clearForwarding(tunIface, extIface string) error {
	cmd := exec.Command("iptables", "-D", "FORWARD", "-i", extIface, "-o", tunIface, "-m", "state", "--state",
		"RELATED,ESTABLISHED", "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove forwarding rule for extIface -> tunIface: %v, output: %s", err, output)
	}

	cmd = exec.Command("iptables", "-D", "FORWARD", "-i", tunIface, "-o", extIface, "-j", "ACCEPT")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove forwarding rule for tunIface -> extIface: %v, output: %s", err, output)
	}
	return nil
}

func getDefaultInterface() (string, error) {
	out, err := exec.Command("ip", "route").Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "default") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				return fields[4], nil
			}
		}
	}
	return "", fmt.Errorf("failed to get default interface")
}
