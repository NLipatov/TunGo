package network

import (
	"encoding/binary"
	"etha-tunnel/network/IPParsing"
	"fmt"
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

	for {
		packet, readFromTunErr := ReadFromTun(tunFile)
		if readFromTunErr != nil {
			log.Printf("failed to read from TUN interface: %v", readFromTunErr)
			continue
		}

		ipHeader, ipV4ParsingErr := IPParsing.ParseIPv4Header(packet)
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
