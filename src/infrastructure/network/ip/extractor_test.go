package ip

import (
	"net/netip"
	"testing"
)

func TestExtractSourceIP_IPv4(t *testing.T) {
	// Build a minimal IPv4 header
	packet := make([]byte, IPv4HeaderMinLen)
	packet[0] = 0x45 // Version 4, IHL 5

	// Source IP at offset 12-15: 192.168.1.100
	packet[12] = 192
	packet[13] = 168
	packet[14] = 1
	packet[15] = 100

	srcIP, ok := ExtractSourceIP(packet)
	if !ok {
		t.Fatal("should successfully extract IPv4 source IP")
	}

	expected := netip.MustParseAddr("192.168.1.100")
	if srcIP != expected {
		t.Fatalf("expected %s, got %s", expected, srcIP)
	}
}

func TestExtractSourceIP_IPv6(t *testing.T) {
	// Build a minimal IPv6 header
	packet := make([]byte, IPv6HeaderLen)
	packet[0] = 0x60 // Version 6

	// Source IP at offset 8-23: 2001:db8::1
	// 2001:0db8:0000:0000:0000:0000:0000:0001
	packet[8] = 0x20
	packet[9] = 0x01
	packet[10] = 0x0d
	packet[11] = 0xb8
	// Bytes 12-22 are zeros
	packet[23] = 0x01

	srcIP, ok := ExtractSourceIP(packet)
	if !ok {
		t.Fatal("should successfully extract IPv6 source IP")
	}

	expected := netip.MustParseAddr("2001:db8::1")
	if srcIP != expected {
		t.Fatalf("expected %s, got %s", expected, srcIP)
	}
}

func TestExtractSourceIP_TooShort(t *testing.T) {
	// Empty packet
	_, ok := ExtractSourceIP([]byte{})
	if ok {
		t.Fatal("should fail for empty packet")
	}

	// IPv4 header too short
	packet := make([]byte, IPv4HeaderMinLen-1)
	packet[0] = 0x45
	_, ok = ExtractSourceIP(packet)
	if ok {
		t.Fatal("should fail for truncated IPv4 header")
	}

	// IPv6 header too short
	packet = make([]byte, IPv6HeaderLen-1)
	packet[0] = 0x60
	_, ok = ExtractSourceIP(packet)
	if ok {
		t.Fatal("should fail for truncated IPv6 header")
	}
}

func TestExtractSourceIP_InvalidVersion(t *testing.T) {
	packet := make([]byte, 20)
	packet[0] = 0x50 // Version 5 (invalid)

	_, ok := ExtractSourceIP(packet)
	if ok {
		t.Fatal("should fail for invalid IP version")
	}
}

func TestExtractDestIP_IPv4(t *testing.T) {
	packet := make([]byte, IPv4HeaderMinLen)
	packet[0] = 0x45 // Version 4, IHL 5

	// Dest IP at offset 16-19: 10.0.0.1
	packet[16] = 10
	packet[17] = 0
	packet[18] = 0
	packet[19] = 1

	dstIP, ok := ExtractDestIP(packet)
	if !ok {
		t.Fatal("should successfully extract IPv4 dest IP")
	}

	expected := netip.MustParseAddr("10.0.0.1")
	if dstIP != expected {
		t.Fatalf("expected %s, got %s", expected, dstIP)
	}
}

func TestExtractDestIP_IPv6(t *testing.T) {
	packet := make([]byte, IPv6HeaderLen)
	packet[0] = 0x60 // Version 6

	// Dest IP at offset 24-39: ::1
	packet[39] = 0x01

	dstIP, ok := ExtractDestIP(packet)
	if !ok {
		t.Fatal("should successfully extract IPv6 dest IP")
	}

	expected := netip.MustParseAddr("::1")
	if dstIP != expected {
		t.Fatalf("expected %s, got %s", expected, dstIP)
	}
}

func TestExtractDestIP_TooShort(t *testing.T) {
	// Empty
	_, ok := ExtractDestIP([]byte{})
	if ok {
		t.Fatal("should fail for empty packet")
	}

	// IPv4 too short
	packet := make([]byte, IPv4HeaderMinLen-1)
	packet[0] = 0x45
	_, ok = ExtractDestIP(packet)
	if ok {
		t.Fatal("should fail for truncated IPv4")
	}
}

func TestExtractIPVersion(t *testing.T) {
	// IPv4
	packet := []byte{0x45}
	if ExtractIPVersion(packet) != 4 {
		t.Fatal("should extract version 4")
	}

	// IPv6
	packet = []byte{0x60}
	if ExtractIPVersion(packet) != 6 {
		t.Fatal("should extract version 6")
	}

	// Empty
	if ExtractIPVersion([]byte{}) != 0 {
		t.Fatal("should return 0 for empty packet")
	}
}

func TestExtractSourceIP_RealIPv4Packet(t *testing.T) {
	// A more realistic IPv4 header (ICMP echo request pattern)
	packet := []byte{
		0x45, 0x00, 0x00, 0x54, // Version/IHL, DSCP, Total Length
		0x00, 0x00, 0x40, 0x00, // ID, Flags, Fragment Offset
		0x40, 0x01, 0x00, 0x00, // TTL, Protocol (ICMP), Checksum
		0xc0, 0xa8, 0x01, 0x64, // Source: 192.168.1.100
		0x08, 0x08, 0x08, 0x08, // Dest: 8.8.8.8
	}

	srcIP, ok := ExtractSourceIP(packet)
	if !ok {
		t.Fatal("should extract source IP")
	}
	if srcIP != netip.MustParseAddr("192.168.1.100") {
		t.Fatalf("wrong source: %s", srcIP)
	}

	dstIP, ok := ExtractDestIP(packet)
	if !ok {
		t.Fatal("should extract dest IP")
	}
	if dstIP != netip.MustParseAddr("8.8.8.8") {
		t.Fatalf("wrong dest: %s", dstIP)
	}
}
