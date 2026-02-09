package ip

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// AllocateServerIP returns the first usable IPv4 address in the given subnet.
func AllocateServerIP(subnet netip.Prefix) (string, error) {
	addr := subnet.Addr()
	if !addr.Is4() {
		return "", fmt.Errorf("only IPv4 supported: %s", subnet)
	}
	// convert base network address to uint32 and add 1
	arr := addr.As4() // [4]byte
	base := binary.BigEndian.Uint32(arr[:])
	// construct server IP as AddrFrom4
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], base+1)
	server := netip.AddrFrom4(b)
	return server.String(), nil
}

// AllocateClientIP returns the IPv4 address for the given client index (0-based)
// within the subnet, skipping network and broadcast addresses.
func AllocateClientIP(subnet netip.Prefix, clientCounter int) (netip.Addr, error) {
	addr := subnet.Addr()
	if !addr.Is4() {
		return netip.Addr{}, fmt.Errorf("only IPv4 supported: %s", subnet)
	}
	arr := addr.As4()
	base := binary.BigEndian.Uint32(arr[:])
	ones, bits := subnet.Bits(), 32
	total := 1 << (bits - ones)
	if clientCounter < 0 || clientCounter >= total-2 {
		return netip.Addr{}, fmt.Errorf("client counter exceeds available addresses in the subnet: %d", clientCounter)
	}
	// calculate client IP
	next := base + 1 + uint32(clientCounter)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], next)
	clientIP := netip.AddrFrom4(b)
	// ensure not network or broadcast
	if clientIP == addr || next == base+uint32(total-1) {
		return netip.Addr{}, fmt.Errorf("generated IP is invalid (network or broadcast address): %s", clientIP)
	}
	return clientIP, nil
}

// ToCIDR combines an IP address with the mask length from the given subnet.
func ToCIDR(subnetCIDR string, addressInSubnet string) (string, error) {
	pref, err := netip.ParsePrefix(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %w", err)
	}
	rip, err := netip.ParseAddr(addressInSubnet)
	if err != nil {
		return "", fmt.Errorf("invalid IP address: %w", err)
	}
	// preserve mask bits
	return fmt.Sprintf("%s/%d", rip.String(), pref.Bits()), nil
}
