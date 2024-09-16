package packages

import (
	"net"
	"testing"
)

func TestParseIPv4Header(t *testing.T) {
	packet := []byte{
		0x45, 0x00, 0x00, 0x54, 0xa6, 0xf2, 0x40, 0x00, 0x40, 0x01, 0xb6, 0xec,
		0xc0, 0xa8, 0x00, 0x68, 0xc0, 0xa8, 0x00, 0x01,
	}

	header, err := ParseIPv4Header(packet)
	if err != nil {
		t.Fatalf("failed to parse IPv4 header: %v", err)
	}

	if header.Version != 4 {
		t.Errorf("expected version 4, got %d", header.Version)
	}

	if header.IHL != 5 {
		t.Errorf("expected IHL 5 (20 bytes), got %d", header.IHL)
	}

	if header.DSCP != 0x00 {
		t.Errorf("expected DSCP 0, got %d", header.DSCP)
	}

	if header.TotalLength != 84 {
		t.Errorf("expected TotalLength 84, got %d", header.TotalLength)
	}

	if header.Identification != 0xa6f2 {
		t.Errorf("expected Identification 0xa6f2, got 0x%x", header.Identification)
	}

	if header.Flags != 2 {
		t.Errorf("expected Flags 2 (Don't Fragment), got %d", header.Flags)
	}

	if header.FragmentOffset != 0 {
		t.Errorf("expected FragmentOffset 0, got %d", header.FragmentOffset)
	}

	if header.TTL != 64 {
		t.Errorf("expected TTL 64, got %d", header.TTL)
	}

	if header.Protocol != 1 {
		t.Errorf("expected Protocol 1 (ICMP), got %d", header.Protocol)
	}

	if header.HeaderChecksum != 0xb6ec {
		t.Errorf("expected HeaderChecksum 0xb6ec, got 0x%x", header.HeaderChecksum)
	}

	expectedSourceIP := net.IPv4(192, 168, 0, 104)
	if !header.SourceIP.Equal(expectedSourceIP) {
		t.Errorf("expected SourceIP %s, got %s", expectedSourceIP, header.SourceIP)
	}

	expectedDestinationIP := net.IPv4(192, 168, 0, 1)
	if !header.DestinationIP.Equal(expectedDestinationIP) {
		t.Errorf("expected DestinationIP %s, got %s", expectedDestinationIP, header.DestinationIP)
	}
}

func TestParseIPv4Header_InvalidLength(t *testing.T) {
	packet := []byte{0x45, 0x00}

	_, err := ParseIPv4Header(packet)
	if err == nil {
		t.Errorf("expected error for invalid packet length, but got none")
	}
}

func TestParseIPv4Header_NotIPv4(t *testing.T) {
	packet := []byte{0x65, 0x00, 0x00, 0x14}

	_, err := ParseIPv4Header(packet)
	if err == nil {
		t.Errorf("expected error for non-packages packet, but got none")
	}
}
